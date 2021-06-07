package plugin

import (
	"context"
	"fmt"
	"net"

	dTypes "github.com/docker/docker/api/types"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/devplayer0/docker-net-dhcp/pkg/udhcpc"
	"github.com/devplayer0/docker-net-dhcp/pkg/util"
)

// CLIOptionsKey is the key used in create network options by the CLI for custom options
const CLIOptionsKey string = "com.docker.network.generic"

// Implementations of the endpoints described in
// https://github.com/moby/libnetwork/blob/master/docs/remote.md

// CreateNetwork "creates" a new DHCP network (just checks if the provided bridge exists and the null IPAM driver is
// used)
func (p *Plugin) CreateNetwork(r CreateNetworkRequest) error {
	log.WithField("options", r.Options).Debug("CreateNetwork options")

	opts, err := decodeOpts(r.Options[util.OptionsKeyGeneric])
	if err != nil {
		return fmt.Errorf("failed to decode network options: %w", err)
	}

	if opts.Bridge == "" {
		return util.ErrBridgeRequired
	}

	for _, d := range r.IPv4Data {
		if d.AddressSpace != "null" || d.Pool != "0.0.0.0/0" {
			return util.ErrIPAM
		}
	}

	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("failed to retrieve list of network interfaces: %w", err)
	}

	nets, err := p.docker.NetworkList(context.Background(), dTypes.NetworkListOptions{})
	if err != nil {
		return fmt.Errorf("failed to retrieve list of networks from Docker: %w", err)
	}

	found := false
	for _, l := range links {
		attrs := l.Attrs()
		if l.Type() != "bridge" || attrs.Name != opts.Bridge {
			continue
		}

		v4Addrs, err := netlink.AddrList(l, unix.AF_INET)
		if err != nil {
			return fmt.Errorf("failed to retrieve IPv4 addresses for %v: %w", attrs.Name, err)
		}
		v6Addrs, err := netlink.AddrList(l, unix.AF_INET6)
		if err != nil {
			return fmt.Errorf("failed to retrieve IPv6 addresses for %v: %w", attrs.Name, err)
		}
		addrs := append(v4Addrs, v6Addrs...)

		// Make sure the addresses on this bridge aren't used by another network
		for _, n := range nets {
			for _, c := range n.IPAM.Config {
				_, cidr, err := net.ParseCIDR(c.Subnet)
				if err != nil {
					return fmt.Errorf("failed to parse subnet %v on Docker network %v: %w", c.Subnet, n.ID, err)
				}

				for _, linkAddr := range addrs {
					if linkAddr.IPNet.Contains(cidr.IP) || cidr.Contains(linkAddr.IP) {
						return util.ErrBridgeUsed
					}
				}
			}
		}
		found = true
		break
	}
	if !found {
		return util.ErrBridgeNotFound
	}

	log.WithFields(log.Fields{
		"network": r.NetworkID,
		"bridge":  opts.Bridge,
		"ipv6":    opts.IPv6,
	}).Info("Network created")

	return nil
}

// DeleteNetwork "deletes" a DHCP network (does nothing, the bridge is managed by the user)
func (p *Plugin) DeleteNetwork(r DeleteNetworkRequest) error {
	log.WithField("network", r.NetworkID).Info("Network deleted")
	return nil
}

func vethPairNames(id string) (string, string) {
	return "dh-" + id[:12], id[:12] + "-dh"
}

func (p *Plugin) netOptions(ctx context.Context, id string) (DHCPNetworkOptions, error) {
	dummy := DHCPNetworkOptions{}

	n, err := p.docker.NetworkInspect(ctx, id, dTypes.NetworkInspectOptions{})
	if err != nil {
		return dummy, fmt.Errorf("failed to get info from Docker: %w", err)
	}

	opts, err := decodeOpts(n.Options)
	if err != nil {
		return dummy, fmt.Errorf("failed to parse options: %w", err)
	}

	return opts, nil
}

// CreateEndpoint creates a veth pair and uses udhcpc to acquire an initial IP address on the container end. Docker will
// move the interface into the container's namespace and apply the address.
func (p *Plugin) CreateEndpoint(ctx context.Context, r CreateEndpointRequest) (CreateEndpointResponse, error) {
	log.WithField("options", r.Options).Debug("CreateEndpoint options")
	res := CreateEndpointResponse{
		Interface: &EndpointInterface{},
	}

	if r.Interface != nil && (r.Interface.Address != "" || r.Interface.AddressIPv6 != "") {
		// TODO: Should we allow static IP's somehow?
		return res, util.ErrIPAM
	}

	opts, err := p.netOptions(ctx, r.NetworkID)
	if err != nil {
		return res, fmt.Errorf("failed to get network options: %w", err)
	}

	bridge, err := netlink.LinkByName(opts.Bridge)
	if err != nil {
		return res, fmt.Errorf("failed to get bridge interface: %w", err)
	}

	hostName, ctrName := vethPairNames(r.EndpointID)
	la := netlink.NewLinkAttrs()
	la.Name = hostName
	hostLink := &netlink.Veth{
		LinkAttrs: la,
		PeerName:  ctrName,
	}
	if r.Interface.MacAddress != "" {
		addr, err := net.ParseMAC(r.Interface.MacAddress)
		if err != nil {
			return res, util.ErrMACAddress
		}

		hostLink.PeerHardwareAddr = addr
	}

	if err := netlink.LinkAdd(hostLink); err != nil {
		return res, fmt.Errorf("failed to create veth pair: %w", err)
	}
	if err := func() error {
		if err := netlink.LinkSetUp(hostLink); err != nil {
			return fmt.Errorf("failed to set host side link of veth pair up: %w", err)
		}

		ctrLink, err := netlink.LinkByName(ctrName)
		if err != nil {
			return fmt.Errorf("failed to find container side of veth pair: %w", err)
		}
		if err := netlink.LinkSetUp(ctrLink); err != nil {
			return fmt.Errorf("failed to set container side link of veth pair up: %w", err)
		}
		if r.Interface.MacAddress == "" {
			// Only write back the MAC address if it wasn't provided to us by libnetwork
			res.Interface.MacAddress = ctrLink.Attrs().HardwareAddr.String()
		}

		if err := netlink.LinkSetMaster(hostLink, bridge); err != nil {
			return fmt.Errorf("failed to attach host side link of veth peer to bridge: %w", err)
		}

		timeout := defaultLeaseTimeout
		if opts.LeaseTimeout != 0 {
			timeout = opts.LeaseTimeout
		}
		initialIP := func(v6 bool) error {
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			info, err := udhcpc.GetIP(timeoutCtx, ctrName, &udhcpc.DHCPClientOptions{V6: v6})
			if err != nil {
				v6str := ""
				if v6 {
					v6str = "v6"
				}
				return fmt.Errorf("failed to get initial IP%v address via DHCP%v: %w", v6str, v6str, err)
			}

			if v6 {
				res.Interface.AddressIPv6 = info.IP
				// No gateways in DHCPv6!
			} else {
				res.Interface.Address = info.IP
				p.gatewayHints[r.EndpointID] = info.Gateway
			}

			return nil
		}

		if err := initialIP(false); err != nil {
			return err
		}
		if opts.IPv6 {
			if err := initialIP(true); err != nil {
				return err
			}
		}

		return nil
	}(); err != nil {
		// Be sure to clean up the veth pair if any of this fails
		netlink.LinkDel(hostLink)
		return res, err
	}

	log.WithFields(log.Fields{
		"network":  r.NetworkID[:12],
		"endpoint": r.EndpointID[:12],
		"ip":       res.Interface.Address,
		"ipv6":     res.Interface.AddressIPv6,
		"hints":    fmt.Sprintf("%#v", p.gatewayHints[r.EndpointID]),
	}).Info("Endpoint created")

	return res, nil
}

// EndpointOperInfo retrieves some info about an existing endpoint
func (p *Plugin) EndpointOperInfo(r InfoRequest) (InfoResponse, error) {
	// TODO: Return some useful information
	return InfoResponse{}, nil
}

// DeleteEndpoint deletes the veth pair
func (p *Plugin) DeleteEndpoint(r DeleteEndpointRequest) error {
	hostName, _ := vethPairNames(r.EndpointID)
	link, err := netlink.LinkByName(hostName)
	if err != nil {
		return fmt.Errorf("failed to lookup host veth interface %v: %w", hostName, err)
	}

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete veth pair: %w", err)
	}

	log.WithFields(log.Fields{
		"network":  r.NetworkID[:12],
		"endpoint": r.EndpointID[:12],
	}).Info("Endpoint deleted")

	return nil
}

// Join passes the veth name and route information (gateway from DHCP and existing routes on the host bridge) to Docker
// and starts a persistent DHCP client to maintain the lease on the acquired IP
func (p *Plugin) Join(ctx context.Context, r JoinRequest) (JoinResponse, error) {
	log.WithField("options", r.Options).Debug("Join options")
	res := JoinResponse{}

	opts, err := p.netOptions(ctx, r.NetworkID)
	if err != nil {
		return res, fmt.Errorf("failed to get network options: %w", err)
	}

	_, ctrName := vethPairNames(r.EndpointID)

	res.InterfaceName = InterfaceName{
		SrcName:   ctrName,
		DstPrefix: opts.Bridge,
	}

	if hint, ok := p.gatewayHints[r.EndpointID]; ok {
		log.WithFields(log.Fields{
			"network":  r.NetworkID[:12],
			"endpoint": r.EndpointID[:12],
			"sandbox":  r.SandboxKey,
			"gateway":  hint,
		}).Info("[Join] Setting IPv4 gateway retrieved from CreateEndpoint")
		res.Gateway = hint

		delete(p.gatewayHints, r.EndpointID)
	}

	// TODO: Try to intelligently copy existing routes from the bridge
	// TODO: Start a persistent DHCP client

	log.WithFields(log.Fields{
		"network":  r.NetworkID[:12],
		"endpoint": r.EndpointID[:12],
		"sandbox":  r.SandboxKey,
	}).Info("Joined sandbox to endpoint")

	return res, nil
}

// Leave stops the persistent DHCP client for an endpoint
func (p *Plugin) Leave(ctx context.Context, r LeaveRequest) error {
	// TODO: Actually stop the DHCP client

	log.WithFields(log.Fields{
		"network":  r.NetworkID[:12],
		"endpoint": r.EndpointID[:12],
	}).Info("Sandbox left endpoint")

	return nil
}

package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	dTypes "github.com/docker/docker/api/types"
)

// CLIOptionsKey is the key used in create network options by the CLI for custom options
const CLIOptionsKey string = "com.docker.network.generic"

var (
	// ErrIPAM indicates an unsupported IPAM driver was used
	ErrIPAM = errors.New("only the null IPAM driver is supported")
	// ErrBridge indicates that a bridge is unavailable for use
	ErrBridge = errors.New("bridge not found or already in use by Docker")
)

// CreateNetwork "creates" a new DHCP network (just checks if the provided bridge exists and the null IPAM driver is
// used)
func (p *Plugin) CreateNetwork(r CreateNetworkRequest) error {
	for _, d := range r.IPv4Data {
		if d.AddressSpace != "null" || d.Pool != "0.0.0.0/0" {
			return ErrIPAM
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
		if l.Type() != "bridge" || attrs.Name != r.Options.Generic.Bridge {
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
						return ErrBridge
					}
				}
			}
		}
		found = true
		break
	}
	if !found {
		return ErrBridge
	}

	log.WithFields(log.Fields{
		"network": r.NetworkID,
		"bridge":  r.Options.Generic.Bridge,
		"ipv6":    r.Options.Generic.IPv6,
	}).Info("Creating network")

	return nil
}

// DeleteNetwork "deletes" a DHCP network (does nothing, the bridge is managed by the user)
func (p *Plugin) DeleteNetwork() error {
	return nil
}

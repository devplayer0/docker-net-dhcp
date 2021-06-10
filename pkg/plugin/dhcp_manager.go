package plugin

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	dTypes "github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/devplayer0/docker-net-dhcp/pkg/udhcpc"
	"github.com/devplayer0/docker-net-dhcp/pkg/util"
)

const pollTime = 100 * time.Millisecond

type dhcpManager struct {
	docker  *docker.Client
	joinReq JoinRequest
	opts    DHCPNetworkOptions

	LastIP   *netlink.Addr
	LastIPv6 *netlink.Addr

	nsPath    string
	hostname  string
	nsHandle  netns.NsHandle
	netHandle *netlink.Handle
	ctrLink   netlink.Link

	stopChan  chan struct{}
	errChan   chan error
	errChanV6 chan error
}

func newDHCPManager(docker *docker.Client, r JoinRequest, opts DHCPNetworkOptions) *dhcpManager {
	return &dhcpManager{
		docker:  docker,
		joinReq: r,
		opts:    opts,

		stopChan: make(chan struct{}),
	}
}

func (m *dhcpManager) logFields(v6 bool) log.Fields {
	return log.Fields{
		"network":  m.joinReq.NetworkID[:12],
		"endpoint": m.joinReq.EndpointID[:12],
		"sandbox":  m.joinReq.SandboxKey,
		"is_ipv6":  v6,
	}
}

func (m *dhcpManager) renew(v6 bool, info udhcpc.Info) error {
	lastIP := m.LastIP
	if v6 {
		lastIP = m.LastIPv6
	}

	ip, err := netlink.ParseAddr(info.IP)
	if err != nil {
		return fmt.Errorf("failed to parse IP address: %w", err)
	}

	if !ip.Equal(*lastIP) {
		// TODO: We can't deal with a different renewed IP for the same reason as described for `bound`
		log.
			WithFields(m.logFields(v6)).
			WithField("old_ip", lastIP).
			WithField("new_ip", ip).
			Warn("udhcpc renew with changed IP")
	}

	if !v6 && info.Gateway != "" {
		newGateway := net.ParseIP(info.Gateway)

		routes, err := m.netHandle.RouteListFiltered(unix.AF_INET, &netlink.Route{
			LinkIndex: m.ctrLink.Attrs().Index,
			Dst:       nil,
		}, netlink.RT_FILTER_OIF|netlink.RT_FILTER_DST)
		if err != nil {
			return fmt.Errorf("failed to list routes: %w", err)
		}

		if len(routes) == 0 {
			log.
				WithFields(m.logFields(v6)).
				WithField("gateway", newGateway).
				Info("udhcpc renew adding default route")

			if err := m.netHandle.RouteAdd(&netlink.Route{
				LinkIndex: m.ctrLink.Attrs().Index,
				Gw:        newGateway,
			}); err != nil {
				return fmt.Errorf("failed to add default route: %w", err)
			}
		} else if !newGateway.Equal(routes[0].Gw) {
			log.
				WithFields(m.logFields(v6)).
				WithField("old_gateway", routes[0].Gw).
				WithField("new_gateway", newGateway).
				Info("udhcpc renew replacing default route")

			routes[0].Gw = newGateway
			if err := m.netHandle.RouteReplace(&routes[0]); err != nil {
				return fmt.Errorf("failed to replace default route: %w", err)
			}
		}
	}

	return nil
}

func (m *dhcpManager) setupClient(v6 bool) (chan error, error) {
	v6Str := ""
	if v6 {
		v6Str = "v6"
	}

	log.
		WithFields(m.logFields(v6)).
		Info("Starting persistent DHCP client")

	client, err := udhcpc.NewDHCPClient(m.ctrLink.Attrs().Name, &udhcpc.DHCPClientOptions{
		Hostname:  m.hostname,
		V6:        v6,
		Namespace: m.nsPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create DHCP%v client: %w", v6Str, err)
	}

	events, err := client.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start DHCP%v client: %w", v6Str, err)
	}

	errChan := make(chan error)
	go func() {
		for {
			select {
			case event := <-events:
				switch event.Type {
				// TODO: We can't really allow the IP in the container to be deleted, it'll delete some of our previously
				// copied routes. Should this be handled somehow?
				//case "deconfig":
				//	ip := m.LastIP
				//	if v6 {
				//		ip = m.LastIPv6
				//	}

				//	log.
				//		WithFields(m.logFields(v6)).
				//		WithField("ip", ip).
				//		Info("udhcpc deconfiguring, deleting previously acquired IP")
				//	if err := m.netHandle.AddrDel(m.ctrLink, ip); err != nil {
				//		log.
				//			WithError(err).
				//			WithFields(m.logFields(v6)).
				//			WithField("ip", ip).
				//			Error("Failed to delete existing udhcpc address")
				//	}
				// We're `bound` from the beginning
				//case "bound":
				case "renew":
					log.
						WithFields(m.logFields(v6)).
						Debug("udhcpc renew")

					if err := m.renew(v6, event.Data); err != nil {
						log.
							WithError(err).
							WithFields(m.logFields(v6)).
							WithField("gateway", event.Data.Gateway).
							WithField("new_ip", event.Data.IP).
							Error("Failed to execute IP renewal")
					}
				case "leasefail":
					log.WithFields(m.logFields(v6)).Warn("udhcpc failed to get a lease")
				case "nak":
					log.WithFields(m.logFields(v6)).Warn("udhcpc client received NAK")
				}

			case <-m.stopChan:
				log.
					WithFields(m.logFields(v6)).
					Info("Shutting down persistent DHCP client")

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				errChan <- client.Finish(ctx)
				return
			}
		}
	}()

	return errChan, nil
}

func (m *dhcpManager) Start(ctx context.Context) error {
	var ctrID string
	if err := util.AwaitCondition(ctx, func() (bool, error) {
		dockerNet, err := m.docker.NetworkInspect(ctx, m.joinReq.NetworkID, dTypes.NetworkInspectOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get Docker network info: %w", err)
		}

		for id, info := range dockerNet.Containers {
			if info.EndpointID == m.joinReq.EndpointID {
				ctrID = id
				break
			}
		}
		if ctrID == "" {
			return false, util.ErrNoContainer
		}

		// Seems like Docker makes the container ID just the endpoint until it's ready
		return !strings.HasPrefix(ctrID, "ep-"), nil
	}, pollTime); err != nil {
		return err
	}

	ctr, err := util.AwaitContainerInspect(ctx, m.docker, ctrID, pollTime)
	if err != nil {
		return fmt.Errorf("failed to get Docker container info: %w", err)
	}

	// Using the "sandbox key" directly causes issues on some platforms
	m.nsPath = fmt.Sprintf("/proc/%v/ns/net", ctr.State.Pid)
	m.hostname = ctr.Config.Hostname

	m.nsHandle, err = util.AwaitNetNS(ctx, m.nsPath, pollTime)
	if err != nil {
		return fmt.Errorf("failed to get sandbox network namespace: %w", err)
	}

	m.netHandle, err = netlink.NewHandleAt(m.nsHandle)
	if err != nil {
		m.nsHandle.Close()
		return fmt.Errorf("failed to open netlink handle in sandbox namespace: %w", err)
	}

	if err := func() error {
		hostName, oldCtrName := vethPairNames(m.joinReq.EndpointID)
		hostLink, err := netlink.LinkByName(hostName)
		if err != nil {
			return fmt.Errorf("failed to find host side of veth pair: %w", err)
		}
		hostVeth, ok := hostLink.(*netlink.Veth)
		if !ok {
			return util.ErrNotVEth
		}

		ctrIndex, err := netlink.VethPeerIndex(hostVeth)
		if err != nil {
			return fmt.Errorf("failed to get container side of veth's index: %w", err)
		}

		if err := util.AwaitCondition(ctx, func() (bool, error) {
			m.ctrLink, err = util.AwaitLinkByIndex(ctx, m.netHandle, ctrIndex, pollTime)
			if err != nil {
				return false, fmt.Errorf("failed to get link for container side of veth pair: %w", err)
			}

			return m.ctrLink.Attrs().Name != oldCtrName, nil
		}, pollTime); err != nil {
			return err
		}

		if m.errChan, err = m.setupClient(false); err != nil {
			close(m.stopChan)
			return err
		}

		if m.opts.IPv6 {
			if m.errChanV6, err = m.setupClient(true); err != nil {
				close(m.stopChan)
				return err
			}
		}

		return nil
	}(); err != nil {
		m.netHandle.Delete()
		m.nsHandle.Close()
		return err
	}

	return nil
}

func (m *dhcpManager) Stop() error {
	defer m.nsHandle.Close()
	defer m.netHandle.Delete()

	close(m.stopChan)

	if err := <-m.errChan; err != nil {
		return fmt.Errorf("failed shut down DHCP client: %w", err)
	}
	if m.opts.IPv6 {
		if err := <-m.errChanV6; err != nil {
			return fmt.Errorf("failed shut down DHCPv6 client: %w", err)
		}
	}

	return nil
}

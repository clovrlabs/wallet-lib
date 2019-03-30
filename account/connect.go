package account

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/breez/breez/data"
	"github.com/breez/lightninglib/lnrpc"
)

var (
	waitConnectTimeout = time.Second * 20
)

type onlineNotifier struct {
	sync.Mutex
	ntfnChan chan struct{}
	isOnline bool
}

func (n *onlineNotifier) connected() bool {
	n.Lock()
	defer n.Unlock()
	return n.isOnline
}

func (n *onlineNotifier) setOffline() {
	n.Lock()
	defer n.Unlock()
	n.ntfnChan = make(chan struct{})
	n.isOnline = false
}

func (n *onlineNotifier) setOnline() {
	n.Lock()
	// prevent calling multiple times to setOnline and causing panic of closing a closed
	// channel.
	var ntfnChan chan struct{}
	if !n.isOnline {
		ntfnChan = n.ntfnChan
	}
	n.isOnline = true
	n.Unlock()
	if ntfnChan != nil {
		close(ntfnChan)
	}
}

func (n *onlineNotifier) notifyWhenOnline() <-chan struct{} {
	n.Lock()
	defer n.Unlock()
	return n.ntfnChan
}

// IsConnectedToRoutingNode returns true if we are connected to the routing node.
func (a *Service) IsConnectedToRoutingNode() bool {
	return a.connectedNotifier.connected()
}

func (a *Service) onRoutingNodeConnection(connected bool) {

	// BREEZ-377: When there is no channel request one from Breez
	if connected {
		a.connectedNotifier.setOnline()
		accData, _ := a.calculateAccount()
		go a.updateNodeChannelPolicy(accData.Id)
		go a.ensureRoutingChannelOpened()
	} else {
		a.connectedNotifier.setOffline()

		// in case we don't have a channel yet, we will try to connect
		// again so we can keep trying to get an opened channel.
		_, channels, _ := a.getBreezOpenChannels()
		if len(channels) == 0 {
			a.connectRoutingNode()
		}
	}
	a.onServiceEvent(data.NotificationEvent{Type: data.NotificationEvent_ROUTING_NODE_CONNECTION_CHANGED})
}

func (a *Service) connectRoutingNode() error {
	a.log.Infof("Connecting to routing node host: %v, pubKey: %v", a.cfg.RoutingNodeHost, a.cfg.RoutingNodePubKey)
	_, err := a.lightningClient.ConnectPeer(context.Background(), &lnrpc.ConnectPeerRequest{
		Addr: &lnrpc.LightningAddress{
			Pubkey: a.cfg.RoutingNodePubKey,
			Host:   a.cfg.RoutingNodeHost,
		},
		Perm: true,
	})
	return err
}

func (a *Service) disconnectRoutingNode() error {
	a.log.Infof("Disconnecting from routing node host: %v, pubKey: %v", a.cfg.RoutingNodeHost, a.cfg.RoutingNodePubKey)
	_, err := a.lightningClient.DisconnectPeer(context.Background(), &lnrpc.DisconnectPeerRequest{
		PubKey: a.cfg.RoutingNodePubKey,
	})
	return err
}

func (a *Service) waitRoutingNodeConnected() error {
	select {
	case <-a.connectedNotifier.notifyWhenOnline():
		return nil
	case <-time.After(waitConnectTimeout):
		return fmt.Errorf("Timeout has exceeded while trying to process your request.")
	}
}

func (a *Service) connectOnStartup() {
	channelPoints, _, err := a.getBreezOpenChannels()
	if err != nil {
		a.log.Errorf("connectOnStartup: error in getBreezOpenChannels", err)
		return
	}
	pendingChannels, err := a.lightningClient.PendingChannels(context.Background(), &lnrpc.PendingChannelsRequest{})
	if err != nil {
		a.log.Errorf("connectOnStartup: error in PendingChannels", err)
		return
	}
	if len(channelPoints) > 0 || len(pendingChannels.PendingOpenChannels) > 0 {
		a.log.Infof("connectOnStartup: already has a channel, ignoring manual connection")
		return
	}

	a.connectRoutingNode()
}
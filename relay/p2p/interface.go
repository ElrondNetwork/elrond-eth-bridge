package p2p

import (
	"github.com/ElrondNetwork/elrond-go-core/core"
	"github.com/ElrondNetwork/elrond-go/p2p"
	erdgoCore "github.com/ElrondNetwork/elrond-sdk-erdgo/core"
)

// NetMessenger is the definition of an entity able to receive and send messages
type NetMessenger interface {
	ID() core.PeerID
	Bootstrap() error
	Addresses() []string
	RegisterMessageProcessor(topic string, identifier string, processor p2p.MessageProcessor) error
	HasTopic(name string) bool
	CreateTopic(name string, createChannelForTopic bool) error
	Broadcast(topic string, buff []byte)
	SendToConnectedPeer(topic string, buff []byte, peerID core.PeerID) error
	Close() error
	IsInterfaceNil() bool
}

// ElrondRoleProvider defines the operations for an Elrond role provider
type ElrondRoleProvider interface {
	IsWhitelisted(address erdgoCore.AddressHandler) bool
	IsInterfaceNil() bool
}

// SignatureProcessor defines the operations needed to process signatures
type SignatureProcessor interface {
	VerifyEthSignature(signature []byte, messageHash []byte) error
	IsInterfaceNil() bool
}

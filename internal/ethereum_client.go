package internal

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	netUrl "net/url"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

//go:generate mockery --output ./mocks --name EthClient
type EthClient interface {
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	PeerCount(ctx context.Context) (uint64, error)
	SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error)
}

type BasicAuthCredentials struct {
	Username string
	Password string
}

type EthClientGetter func(url string, credentials *BasicAuthCredentials) (EthClient, error)

func NewEthClient(url string, credentials *BasicAuthCredentials) (EthClient, error) {
	if credentials == nil {
		return ethclient.Dial(url)
	}

	parsedURL, err := netUrl.Parse(url)
	if err != nil {
		return nil, err
	}

	// Depending on the scheme, ethclient has different ways of setting auth
	// headers. Using the ethclient is starting to get a bit hacky, as we need to
	// understand the internals to create clients with the auth headers we want.
	// Consider implementing our own client if we need to hack around it more.
	switch parsedURL.Scheme {
	case "http", "https":
		c, err := rpc.DialContext(context.Background(), url)

		if err != nil {
			return nil, err
		}

		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials.Username + ":" + credentials.Password))
		c.SetHeader("Authorization", "Basic "+encodedCredentials)

		return ethclient.NewClient(c), nil
	case "ws", "wss":
		parsedURL.User = netUrl.UserPassword(credentials.Username, credentials.Password)
		urlWithUser := parsedURL.String()
		c, err := rpc.DialContext(context.Background(), urlWithUser)

		if err != nil {
			return nil, err
		}

		return ethclient.NewClient(c), nil
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}
}

package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	netUrl "net/url"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/satsuma-data/node-gateway/internal/config"
)

const (
	clientDialTimeout = 10 * time.Second
)

type NewHeadHandler struct {
	OnNewHead func(header *types.Header)
	OnError   func(failure string)
}

//go:generate mockery --output ../mocks --name EthClient --with-expecter
type EthClient interface {
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	PeerCount(ctx context.Context) (uint64, error)
	SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error)
}

type EthClientGetter func(url string, credentials *config.BasicAuthConfig, additionalRequestHeaders *[]config.RequestHeaderConfig) (EthClient, error)

func NewEthClient(url string, credentials *config.BasicAuthConfig, additionalRequestHeaders *[]config.RequestHeaderConfig) (EthClient, error) {
	rpcClient, err := getRPCClientWithAuthHeader(url, credentials)
	if err != nil {
		return nil, err
	}

	setAdditionalRequestHeaders(rpcClient, additionalRequestHeaders)

	return ethclient.NewClient(rpcClient), nil
}

func getRPCClientWithAuthHeader(url string, credentials *config.BasicAuthConfig) (*rpc.Client, error) {
	if credentials == nil || (credentials.Username == "" && credentials.Password == "") {
		ctx, cancel := context.WithTimeout(context.Background(), clientDialTimeout)
		defer cancel()

		return rpc.DialContext(ctx, url)
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
		ctx, cancel := context.WithTimeout(context.Background(), clientDialTimeout)
		defer cancel()

		rpcClient, err := rpc.DialContext(ctx, url)

		if err != nil {
			return nil, err
		}

		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials.Username + ":" + credentials.Password))
		rpcClient.SetHeader("Authorization", "Basic "+encodedCredentials)

		return rpcClient, nil
	case "ws", "wss":
		parsedURL.User = netUrl.UserPassword(credentials.Username, credentials.Password)
		urlWithUser := parsedURL.String()

		ctx, cancel := context.WithTimeout(context.Background(), clientDialTimeout)
		defer cancel()

		return rpc.DialContext(ctx, urlWithUser)
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}
}

func setAdditionalRequestHeaders(c *rpc.Client, additionalRequestHeaders *[]config.RequestHeaderConfig) {
	for _, requestHeader := range *additionalRequestHeaders {
		c.SetHeader(requestHeader.Key, requestHeader.Value)
	}
}

package gasManagement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/ElrondNetwork/elrond-eth-bridge/core"
	"github.com/ElrondNetwork/elrond-go-core/core/atomic"
	logger "github.com/ElrondNetwork/elrond-go-logger"
)

const minPollingInterval = time.Second
const minRequestTime = time.Millisecond
const logPath = "EthClient/gasStation"

// ArgsGasStation is the DTO used for the creating a new gas handler instance
type ArgsGasStation struct {
	RequestURL             string
	RequestPollingInterval time.Duration
	RequestTime            time.Duration
	MaximumGasPrice        int
	GasPriceSelector       core.EthGasPriceSelector
}

type gasStation struct {
	requestURL             string
	requestTime            time.Duration
	requestPollingInterval time.Duration
	log                    logger.Logger
	httpClient             HTTPClient
	maximumGasPrice        int
	cancel                 func()
	gasPriceSelector       core.EthGasPriceSelector
	loopStatus             *atomic.Flag

	mut            sync.RWMutex
	latestResponse *gasStationResponse
}

// NewGasStation returns a new gas handler instance for the gas station service
func NewGasStation(args ArgsGasStation) (*gasStation, error) {
	err := checkArgs(args)
	if err != nil {
		return nil, err
	}

	gs := &gasStation{
		requestURL:             args.RequestURL,
		requestTime:            args.RequestTime,
		requestPollingInterval: args.RequestPollingInterval,
		httpClient:             http.DefaultClient,
		maximumGasPrice:        args.MaximumGasPrice,
		gasPriceSelector:       args.GasPriceSelector,
		loopStatus:             &atomic.Flag{},
	}
	gs.log = logger.GetOrCreate(logPath)
	ctx, cancel := context.WithCancel(context.Background())
	gs.cancel = cancel
	go gs.processLoop(ctx)

	return gs, nil
}

func checkArgs(args ArgsGasStation) error {
	if args.RequestPollingInterval < minPollingInterval {
		return fmt.Errorf("%w in checkArgs for value RequestPollingInterval", ErrInvalidValue)
	}
	if args.RequestTime < minRequestTime {
		return fmt.Errorf("%w in checkArgs for value RequestTime", ErrInvalidValue)
	}

	switch args.GasPriceSelector {
	case core.EthFastGasPrice, core.EthFastestGasPrice, core.EthSafeLowGasPrice, core.EthAverageGasPrice:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidGasPriceSelector, args.GasPriceSelector)
	}

	return nil
}

func (gs *gasStation) processLoop(ctx context.Context) {
	gs.loopStatus.Set()
	defer gs.loopStatus.Unset()

	for {
		requestContext, cancel := context.WithTimeout(ctx, gs.requestTime)

		err := gs.doRequest(requestContext)
		if err != nil {
			gs.log.Error("gasHandler.processLoop", "error", err.Error())
		}
		cancel()

		select {
		case <-ctx.Done():
			gs.log.Debug("Ethereum's gas station fetcher main execute loop is closing...")
			return
		case <-time.After(gs.requestPollingInterval):
		}
	}
}

func (gs *gasStation) doRequest(ctx context.Context) error {
	bytes, err := gs.doRequestReturningBytes(ctx)
	if err != nil {
		return err
	}

	response := &gasStationResponse{}
	err = json.Unmarshal(bytes, response)
	if err != nil {
		return err
	}

	gs.log.Debug("gas station: fetched new response", "response data", response)

	gs.mut.Lock()
	gs.latestResponse = response
	gs.mut.Unlock()

	return nil
}

func (gs *gasStation) doRequestReturningBytes(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gs.requestURL, nil)
	if err != nil {
		return nil, err
	}

	response, err := gs.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// GetCurrentGasPrice will return the read value from the last query carried on the service provider
// It errors if the gas price values were not fetched from the service provider or the fetched value
// exceeds the maximum gas price provided
func (gs *gasStation) GetCurrentGasPrice() (int, error) {
	gs.mut.RLock()
	defer gs.mut.RUnlock()

	if gs.latestResponse == nil {
		return 0, ErrLatestGasPricesWereNotFetched
	}

	gasPrice := 0
	switch gs.gasPriceSelector {
	case core.EthFastGasPrice:
		gasPrice = gs.latestResponse.Fast
	case core.EthFastestGasPrice:
		gasPrice = gs.latestResponse.Fastest
	case core.EthSafeLowGasPrice:
		gasPrice = gs.latestResponse.SafeLow
	case core.EthAverageGasPrice:
		gasPrice = gs.latestResponse.Average
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidGasPriceSelector, gs.gasPriceSelector)
	}

	if gasPrice > gs.maximumGasPrice {
		return 0, fmt.Errorf("%w maximum value: %d, fetched value: %d, gas price selector: %s",
			ErrGasPriceIsHigherThanTheMaximumSet, gs.maximumGasPrice, gasPrice, gs.gasPriceSelector)
	}

	return gasPrice, nil
}

// Close will stop any started go routines
func (gs *gasStation) Close() error {
	gs.cancel()

	return nil
}

// IsInterfaceNil returns true if there is no value under the interface
func (gs *gasStation) IsInterfaceNil() bool {
	return gs == nil
}
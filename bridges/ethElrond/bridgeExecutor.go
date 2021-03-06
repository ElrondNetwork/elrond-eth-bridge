package ethElrond

import (
	"context"
	"fmt"
	"time"

	"github.com/ElrondNetwork/elrond-eth-bridge/clients"
	"github.com/ElrondNetwork/elrond-eth-bridge/core"
	"github.com/ElrondNetwork/elrond-go-core/core/check"
	logger "github.com/ElrondNetwork/elrond-go-logger"
	"github.com/ethereum/go-ethereum/common"
)

// splits - represent the number of times we split the maximum interval
// we wait for the transfer confirmation on Ethereum
const splits = 10

const minRetries = 1

// ArgsBridgeExecutor is the arguments DTO struct used in both bridges
type ArgsBridgeExecutor struct {
	Log                        logger.Logger
	TopologyProvider           TopologyProvider
	ElrondClient               ElrondClient
	EthereumClient             EthereumClient
	TimeForWaitOnEthereum      time.Duration
	StatusHandler              core.StatusHandler
	SignaturesHolder           SignaturesHolder
	BatchValidator             clients.BatchValidator
	MaxQuorumRetriesOnEthereum uint64
	MaxQuorumRetriesOnElrond   uint64
	MaxRestriesOnWasProposed   uint64
}

type bridgeExecutor struct {
	log                        logger.Logger
	topologyProvider           TopologyProvider
	elrondClient               ElrondClient
	ethereumClient             EthereumClient
	timeForWaitOnEthereum      time.Duration
	statusHandler              core.StatusHandler
	sigsHolder                 SignaturesHolder
	batchValidator             clients.BatchValidator
	maxQuorumRetriesOnEthereum uint64
	maxQuorumRetriesOnElrond   uint64
	maxRetriesOnWasProposed    uint64

	batch                   *clients.TransferBatch
	actionID                uint64
	msgHash                 common.Hash
	quorumRetriesOnEthereum uint64
	quorumRetriesOnElrond   uint64
	retriesOnWasProposed    uint64
}

// NewBridgeExecutor creates a bridge executor, which can be used for both half-bridges
func NewBridgeExecutor(args ArgsBridgeExecutor) (*bridgeExecutor, error) {
	err := checkArgs(args)
	if err != nil {
		return nil, err
	}

	executor := createBridgeExecutor(args)
	return executor, nil
}

func checkArgs(args ArgsBridgeExecutor) error {
	if check.IfNil(args.Log) {
		return ErrNilLogger
	}
	if check.IfNil(args.ElrondClient) {
		return ErrNilElrondClient
	}
	if check.IfNil(args.EthereumClient) {
		return ErrNilEthereumClient
	}
	if check.IfNil(args.TopologyProvider) {
		return ErrNilTopologyProvider
	}
	if check.IfNil(args.StatusHandler) {
		return ErrNilStatusHandler
	}
	if args.TimeForWaitOnEthereum < durationLimit {
		return ErrInvalidDuration
	}
	if check.IfNil(args.SignaturesHolder) {
		return ErrNilSignaturesHolder
	}
	if check.IfNil(args.BatchValidator) {
		return ErrNilBatchValidator
	}
	if args.MaxQuorumRetriesOnEthereum < minRetries {
		return fmt.Errorf("%w for args.MaxQuorumRetriesOnEthereum, got: %d, minimum: %d",
			clients.ErrInvalidValue, args.MaxQuorumRetriesOnEthereum, minRetries)
	}
	if args.MaxQuorumRetriesOnElrond < minRetries {
		return fmt.Errorf("%w for args.MaxQuorumRetriesOnElrond, got: %d, minimum: %d",
			clients.ErrInvalidValue, args.MaxQuorumRetriesOnElrond, minRetries)
	}
	if args.MaxRestriesOnWasProposed < minRetries {
		return fmt.Errorf("%w for args.MaxRestriesOnWasProposed, got: %d, minimum: %d",
			clients.ErrInvalidValue, args.MaxRestriesOnWasProposed, minRetries)
	}
	return nil
}

func createBridgeExecutor(args ArgsBridgeExecutor) *bridgeExecutor {
	return &bridgeExecutor{
		log:                        args.Log,
		elrondClient:               args.ElrondClient,
		ethereumClient:             args.EthereumClient,
		topologyProvider:           args.TopologyProvider,
		statusHandler:              args.StatusHandler,
		timeForWaitOnEthereum:      args.TimeForWaitOnEthereum,
		sigsHolder:                 args.SignaturesHolder,
		batchValidator:             args.BatchValidator,
		maxQuorumRetriesOnEthereum: args.MaxQuorumRetriesOnEthereum,
		maxQuorumRetriesOnElrond:   args.MaxQuorumRetriesOnElrond,
		maxRetriesOnWasProposed:    args.MaxRestriesOnWasProposed,
	}
}

// PrintInfo will print the provided data through the inner logger instance
func (executor *bridgeExecutor) PrintInfo(logLevel logger.LogLevel, message string, extras ...interface{}) {
	executor.log.Log(logLevel, message, extras...)

	switch logLevel {
	case logger.LogWarning, logger.LogError:
		executor.setExecutionMessageInStatusHandler(logLevel, message, extras...)
	}
}

func (executor *bridgeExecutor) setExecutionMessageInStatusHandler(level logger.LogLevel, message string, extras ...interface{}) {
	msg := fmt.Sprintf("%s: %s", level, message)
	for i := 0; i < len(extras)-1; i += 2 {
		msg += fmt.Sprintf(" %s = %s", convertObjectToString(extras[i]), convertObjectToString(extras[i+1]))
	}

	executor.statusHandler.SetStringMetric(core.MetricLastError, msg)
}

// MyTurnAsLeader returns true if the current relayer node is the leader
func (executor *bridgeExecutor) MyTurnAsLeader() bool {
	return executor.topologyProvider.MyTurnAsLeader()
}

// GetBatchFromElrond fetches the pending batch from Elrond
func (executor *bridgeExecutor) GetBatchFromElrond(ctx context.Context) (*clients.TransferBatch, error) {
	batch, err := executor.elrondClient.GetPending(ctx)
	if err == nil {
		executor.statusHandler.SetIntMetric(core.MetricNumBatches, int(batch.ID)-1)
	}
	return batch, err
}

// StoreBatchFromElrond saves the pending batch from Elrond
func (executor *bridgeExecutor) StoreBatchFromElrond(batch *clients.TransferBatch) error {
	if batch == nil {
		return ErrNilBatch
	}

	executor.batch = batch
	return nil
}

// GetStoredBatch returns the stored batch
func (executor *bridgeExecutor) GetStoredBatch() *clients.TransferBatch {
	return executor.batch
}

// GetLastExecutedEthBatchIDFromElrond returns the last executed batch ID that is stored on the Elrond SC
func (executor *bridgeExecutor) GetLastExecutedEthBatchIDFromElrond(ctx context.Context) (uint64, error) {
	batchID, err := executor.elrondClient.GetLastExecutedEthBatchID(ctx)
	if err == nil {
		executor.statusHandler.SetIntMetric(core.MetricNumBatches, int(batchID))
	}
	return batchID, err
}

// VerifyLastDepositNonceExecutedOnEthereumBatch will check the deposit nonces from the fetched batch from Ethereum client
func (executor *bridgeExecutor) VerifyLastDepositNonceExecutedOnEthereumBatch(ctx context.Context) error {
	if executor.batch == nil {
		return ErrNilBatch
	}

	lastNonce, err := executor.elrondClient.GetLastExecutedEthTxID(ctx)
	if err != nil {
		return err
	}

	return executor.verifyDepositNonces(lastNonce)
}

func (executor *bridgeExecutor) verifyDepositNonces(lastNonce uint64) error {
	startNonce := lastNonce + 1
	for _, dt := range executor.batch.Deposits {
		if dt.Nonce != startNonce {
			return fmt.Errorf("%w for deposit %s, expected: %d", ErrInvalidDepositNonce, dt.String(), startNonce)
		}

		startNonce++
	}

	return nil
}

// GetAndStoreActionIDForProposeTransferOnElrond fetches the action ID for ProposeTransfer by using the stored batch. Stores the action ID and returns it
func (executor *bridgeExecutor) GetAndStoreActionIDForProposeTransferOnElrond(ctx context.Context) (uint64, error) {
	if executor.batch == nil {
		return InvalidActionID, ErrNilBatch
	}

	actionID, err := executor.elrondClient.GetActionIDForProposeTransfer(ctx, executor.batch)
	if err != nil {
		return InvalidActionID, err
	}

	executor.actionID = actionID

	return actionID, nil
}

// GetAndStoreActionIDForProposeSetStatusFromElrond fetches the action ID for SetStatus by using the stored batch. Stores the action ID and returns it
func (executor *bridgeExecutor) GetAndStoreActionIDForProposeSetStatusFromElrond(ctx context.Context) (uint64, error) {
	if executor.batch == nil {
		return InvalidActionID, ErrNilBatch
	}

	actionID, err := executor.elrondClient.GetActionIDForSetStatusOnPendingTransfer(ctx, executor.batch)
	if err != nil {
		return InvalidActionID, err
	}

	executor.actionID = actionID

	return actionID, nil
}

// GetStoredActionID returns the stored action ID
func (executor *bridgeExecutor) GetStoredActionID() uint64 {
	return executor.actionID
}

// WasTransferProposedOnElrond checks if the transfer was proposed on Elrond
func (executor *bridgeExecutor) WasTransferProposedOnElrond(ctx context.Context) (bool, error) {
	if executor.batch == nil {
		return false, ErrNilBatch
	}

	return executor.elrondClient.WasProposedTransfer(ctx, executor.batch)
}

// ProposeTransferOnElrond propose the transfer on Elrond
func (executor *bridgeExecutor) ProposeTransferOnElrond(ctx context.Context) error {
	if executor.batch == nil {
		return ErrNilBatch
	}

	hash, err := executor.elrondClient.ProposeTransfer(ctx, executor.batch)
	if err != nil {
		return err
	}

	executor.log.Info("proposed transfer", "hash", hash,
		"batch ID", executor.batch.ID, "action ID", executor.actionID)

	return nil
}

// ProcessMaxRetriesOnWasTransferProposedOnElrond checks if the retries on Elrond were reached and increments the counter
func (executor *bridgeExecutor) ProcessMaxRetriesOnWasTransferProposedOnElrond() bool {
	if executor.retriesOnWasProposed < executor.maxRetriesOnWasProposed {
		executor.retriesOnWasProposed++
		return false
	}

	return true
}

// ResetRetriesOnWasTransferProposedOnElrond resets the number of retries on was transfer proposed
func (executor *bridgeExecutor) ResetRetriesOnWasTransferProposedOnElrond() {
	executor.retriesOnWasProposed = 0
}

// WasSetStatusProposedOnElrond checks if set status was proposed on Elrond
func (executor *bridgeExecutor) WasSetStatusProposedOnElrond(ctx context.Context) (bool, error) {
	if executor.batch == nil {
		return false, ErrNilBatch
	}

	return executor.elrondClient.WasProposedSetStatus(ctx, executor.batch)
}

// ProposeSetStatusOnElrond propose set status on Elrond
func (executor *bridgeExecutor) ProposeSetStatusOnElrond(ctx context.Context) error {
	if executor.batch == nil {
		return ErrNilBatch
	}

	hash, err := executor.elrondClient.ProposeSetStatus(ctx, executor.batch)
	if err != nil {
		return err
	}

	executor.log.Info("proposed set status", "hash", hash,
		"batch ID", executor.batch.ID)

	return nil
}

// WasActionSignedOnElrond returns true if the current relayer already signed the action
func (executor *bridgeExecutor) WasActionSignedOnElrond(ctx context.Context) (bool, error) {
	return executor.elrondClient.WasSigned(ctx, executor.actionID)
}

// SignActionOnElrond calls the Elrond client to generate and send the signature
func (executor *bridgeExecutor) SignActionOnElrond(ctx context.Context) error {
	hash, err := executor.elrondClient.Sign(ctx, executor.actionID)
	if err != nil {
		return err
	}

	executor.log.Info("signed proposed transfer", "hash", hash, "action ID", executor.actionID)

	return nil
}

// ProcessQuorumReachedOnElrond returns true if the proposed transfer reached the set quorum
func (executor *bridgeExecutor) ProcessQuorumReachedOnElrond(ctx context.Context) (bool, error) {
	return executor.elrondClient.QuorumReached(ctx, executor.actionID)
}

// WaitForTransferConfirmation waits for the confirmation of a transfer
func (executor *bridgeExecutor) WaitForTransferConfirmation(ctx context.Context) {
	wasPerformed := false
	for i := 0; i < splits && !wasPerformed; i++ {
		if executor.waitWithContextSucceeded(ctx) {
			wasPerformed, _ = executor.WasTransferPerformedOnEthereum(ctx)
		}
	}
}

// WaitAndReturnFinalBatchStatuses waits for the statuses to be final
func (executor *bridgeExecutor) WaitAndReturnFinalBatchStatuses(ctx context.Context) []byte {
	for i := 0; i < splits; i++ {
		if !executor.waitWithContextSucceeded(ctx) {
			return nil
		}

		statuses, err := executor.GetBatchStatusesFromEthereum(ctx)
		if err != nil {
			executor.log.Debug("got message while fetching batch statuses", "message", err)
			continue
		}
		if len(statuses) == 0 {
			executor.log.Debug("no status available")
			continue
		}

		executor.log.Debug("bridgeExecutor.WaitAndReturnFinalBatchStatuses", "statuses", statuses)
		return statuses
	}

	return nil
}

func (executor *bridgeExecutor) waitWithContextSucceeded(ctx context.Context) bool {
	timer := time.NewTimer(executor.timeForWaitOnEthereum / splits)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		executor.log.Debug("closing due to context expiration")
		return false
	case <-timer.C:
		return true
	}
}

// GetBatchStatusesFromEthereum gets statuses for the batch
func (executor *bridgeExecutor) GetBatchStatusesFromEthereum(ctx context.Context) ([]byte, error) {
	if executor.batch == nil {
		return nil, ErrNilBatch
	}

	statuses, err := executor.ethereumClient.GetTransactionsStatuses(ctx, executor.batch.ID)
	if err != nil {
		return nil, err
	}

	return statuses, nil
}

// WasActionPerformedOnElrond returns true if the action was already performed
func (executor *bridgeExecutor) WasActionPerformedOnElrond(ctx context.Context) (bool, error) {
	return executor.elrondClient.WasExecuted(ctx, executor.actionID)
}

// PerformActionOnElrond sends the perform-action transaction on the Elrond chain
func (executor *bridgeExecutor) PerformActionOnElrond(ctx context.Context) error {
	if executor.batch == nil {
		return ErrNilBatch
	}

	hash, err := executor.elrondClient.PerformAction(ctx, executor.actionID, executor.batch)
	if err != nil {
		return err
	}

	executor.log.Info("sent perform action transaction", "hash", hash,
		"batch ID", executor.batch.ID, "action ID", executor.actionID)

	return nil
}

// ResolveNewDepositsStatuses resolves the new deposits statuses for batch
func (executor *bridgeExecutor) ResolveNewDepositsStatuses(numDeposits uint64) {
	executor.batch.ResolveNewDeposits(int(numDeposits))
}

// ProcessMaxQuorumRetriesOnElrond checks if the retries on Elrond were reached and increments the counter
func (executor *bridgeExecutor) ProcessMaxQuorumRetriesOnElrond() bool {
	if executor.quorumRetriesOnElrond < executor.maxQuorumRetriesOnElrond {
		executor.quorumRetriesOnElrond++
		return false
	}

	return true
}

// ResetRetriesCountOnElrond resets the number of retries on Elrond
func (executor *bridgeExecutor) ResetRetriesCountOnElrond() {
	executor.quorumRetriesOnElrond = 0
}

// GetAndStoreBatchFromEthereum fetches and stores the batch from the ethereum client
func (executor *bridgeExecutor) GetAndStoreBatchFromEthereum(ctx context.Context, nonce uint64) error {
	batch, err := executor.ethereumClient.GetBatch(ctx, nonce)
	if err != nil {
		return err
	}

	isBatchInvalid := batch.ID != nonce || len(batch.Deposits) == 0
	if isBatchInvalid {
		return fmt.Errorf("%w, requested nonce: %d, fetched nonce: %d, num deposits: %d",
			ErrBatchNotFound, nonce, batch.ID, len(batch.Deposits))
	}

	executor.batch = batch

	return nil
}

// WasTransferPerformedOnEthereum returns true if the batch was performed on Ethereum
func (executor *bridgeExecutor) WasTransferPerformedOnEthereum(ctx context.Context) (bool, error) {
	if executor.batch == nil {
		return false, ErrNilBatch
	}

	return executor.ethereumClient.WasExecuted(ctx, executor.batch.ID)
}

// SignTransferOnEthereum generates the message hash for batch and broadcast the signature
func (executor *bridgeExecutor) SignTransferOnEthereum() error {
	if executor.batch == nil {
		return ErrNilBatch
	}

	hash, err := executor.ethereumClient.GenerateMessageHash(executor.batch)
	if err != nil {
		return err
	}

	executor.log.Info("generated message hash on Ethereum", "hash", hash,
		"batch ID", executor.batch.ID)

	executor.msgHash = hash
	executor.ethereumClient.BroadcastSignatureForMessageHash(hash)
	return nil
}

// PerformTransferOnEthereum transfers a batch to Ethereum
func (executor *bridgeExecutor) PerformTransferOnEthereum(ctx context.Context) error {
	if executor.batch == nil {
		return ErrNilBatch
	}

	quorumSize, err := executor.ethereumClient.GetQuorumSize(ctx)
	if err != nil {
		return err
	}

	executor.log.Debug("fetched quorum size", "quorum", quorumSize.Int64())

	hash, err := executor.ethereumClient.ExecuteTransfer(ctx, executor.msgHash, executor.batch, int(quorumSize.Int64()))
	if err != nil {
		return err
	}

	executor.log.Info("sent execute transfer", "hash", hash,
		"batch ID", executor.batch.ID)

	return nil
}

// ProcessQuorumReachedOnEthereum returns true if the proposed transfer reached the set quorum
func (executor *bridgeExecutor) ProcessQuorumReachedOnEthereum(ctx context.Context) (bool, error) {
	return executor.ethereumClient.IsQuorumReached(ctx, executor.msgHash)
}

// ProcessMaxQuorumRetriesOnEthereum checks if the retries on Ethereum were reached and increments the counter
func (executor *bridgeExecutor) ProcessMaxQuorumRetriesOnEthereum() bool {
	if executor.quorumRetriesOnEthereum < executor.maxQuorumRetriesOnEthereum {
		executor.quorumRetriesOnEthereum++
		return false
	}

	return true
}

// ResetRetriesCountOnEthereum resets the number of retries on Ethereum
func (executor *bridgeExecutor) ResetRetriesCountOnEthereum() {
	executor.quorumRetriesOnEthereum = 0
}

// ClearStoredP2PSignaturesForEthereum deletes all stored P2P signatures used for Ethereum client
func (executor *bridgeExecutor) ClearStoredP2PSignaturesForEthereum() {
	executor.sigsHolder.ClearStoredSignatures()
	executor.log.Info("cleared stored P2P signatures")
}

// ValidateBatch returns true if the given batch is validated on microservice side
func (executor *bridgeExecutor) ValidateBatch(ctx context.Context, batch *clients.TransferBatch) (bool, error) {
	return executor.batchValidator.ValidateBatch(ctx, batch)
}

// CheckElrondClientAvailability trigger a self availability check for the elrond client
func (executor *bridgeExecutor) CheckElrondClientAvailability(ctx context.Context) error {
	return executor.elrondClient.CheckClientAvailability(ctx)
}

// CheckEthereumClientAvailability trigger a self availability check for the ethereum client
func (executor *bridgeExecutor) CheckEthereumClientAvailability(ctx context.Context) error {
	return executor.ethereumClient.CheckClientAvailability(ctx)
}

// IsInterfaceNil returns true if there is no value under the interface
func (executor *bridgeExecutor) IsInterfaceNil() bool {
	return executor == nil
}

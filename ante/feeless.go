package ante

import (
	"errors"
	"math"
	"strings"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	oraclekeeper "github.com/kiichain/kiichain/v4/x/oracle/keeper"
	oracletypes "github.com/kiichain/kiichain/v4/x/oracle/types"
)

// FeelessDecorator allows specific transactions (e.g., oracle votes) to be executed without deducting fees.
type FeelessDecorator struct {
	feeDecorator sdk.AnteDecorator
	oracleKeeper *oraclekeeper.Keeper
}

// Ensure FeelessDecorator satisfies sdk.AnteDecorator interface
var _ sdk.AnteDecorator = FeelessDecorator{}

// NewFeelessDecorator returns a new instance of FeelessDecorator
func NewFeelessDecorator(feeDecorator sdk.AnteDecorator, oracleKeeper *oraclekeeper.Keeper) FeelessDecorator {
	return FeelessDecorator{
		feeDecorator: feeDecorator,
		oracleKeeper: oracleKeeper,
	}
}

// AnteHandle implements fee skipping logic for feeless transactions
func (fd FeelessDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	isFeeless, err := fd.IsTxFeeless(ctx, tx)
	if err != nil {
		return ctx, err
	}

	if isFeeless {
		// Set maximum priority so the transaction is still processed fast
		ctx = ctx.WithPriority(math.MaxInt64)
		return next(ctx, tx, simulate)
	}

	// Otherwise, proceed with standard fee deduction
	return fd.feeDecorator.AnteHandle(ctx, tx, simulate, next)
}

// IsTxFeeless determines whether the transaction qualifies as feeless
func (fd FeelessDecorator) IsTxFeeless(ctx sdk.Context, tx sdk.Tx) (bool, error) {
	// Disallow multi-message feeless transactions for spam protection
	if len(tx.GetMsgs()) != 1 {
		return false, nil
	}

	// Evaluate the message type
	for _, msg := range tx.GetMsgs() {
		switch m := msg.(type) {
		case *oracletypes.MsgAggregateExchangeRateVote:
			return fd.MsgAggregateExchangeRateVoteIsFeeless(ctx, m)
		default:
			return false, nil
		}
	}

	return false, nil
}

// MsgAggregateExchangeRateVoteIsFeeless returns true if the vote is valid and not already submitted
func (fd FeelessDecorator) MsgAggregateExchangeRateVoteIsFeeless(ctx sdk.Context, msg *oracletypes.MsgAggregateExchangeRateVote) (bool, error) {
	// Decode feeder address
	feederAddr, err := sdk.AccAddressFromBech32(msg.Feeder)
	if err != nil {
		return false, err
	}

	// Decode validator address
	valAddr, err := sdk.ValAddressFromBech32(msg.Validator)
	if err != nil {
		return false, err
	}

	// Check if feeder is authorized for the validator
	if err := fd.oracleKeeper.ValidateFeeder(ctx, feederAddr, valAddr); err != nil {
		return false, err
	}

	// Check if the validator has already submitted a vote
	_, err = fd.oracleKeeper.AggregateExchangeRateVote.Get(ctx, valAddr)

	// If not found, then this is a new vote => feeless
	if err != nil && errors.Is(err, collections.ErrNotFound) {
		return true, nil
	}

	// Otherwise, it's either already voted or an unexpected error
	return false, err
}

package keeper

import (
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

// SendKeeper defines a module interface that facilitates the transfer of coins
// between accounts without the possibility of creating coins.
type SendKeeper interface {
	ViewKeeper

	InputOutputCoins(ctx sdk.Context, inputs []types.Input, outputs []types.Output) error
	SendCoins(ctx sdk.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error

	GetParams(ctx sdk.Context) types.Params
	SetParams(ctx sdk.Context, params types.Params)

	IsSendEnabledCoin(ctx sdk.Context, coin sdk.Coin) bool
	IsSendEnabledCoins(ctx sdk.Context, coins ...sdk.Coin) error

	BlockedAddr(addr sdk.AccAddress) bool
}

var _ SendKeeper = (*BaseSendKeeper)(nil)

// BaseSendKeeper only allows transfers between accounts without the possibility of
// creating coins. It implements the SendKeeper interface.
type BaseSendKeeper struct {
	BaseViewKeeper

	cdc        codec.BinaryCodec
	ak         types.AccountKeeper
	storeKey   sdk.StoreKey
	paramSpace paramtypes.Subspace

	// list of addresses that are restricted from receiving transactions
	blockedAddrs map[string]bool
}

func NewBaseSendKeeper(
	cdc codec.BinaryCodec, storeKey sdk.StoreKey, ak types.AccountKeeper, paramSpace paramtypes.Subspace, blockedAddrs map[string]bool,
) BaseSendKeeper {

	return BaseSendKeeper{
		BaseViewKeeper: NewBaseViewKeeper(cdc, storeKey, ak),
		cdc:            cdc,
		ak:             ak,
		storeKey:       storeKey,
		paramSpace:     paramSpace,
		blockedAddrs:   blockedAddrs,
	}
}

// GetParams returns the total set of bank parameters.
func (k BaseSendKeeper) GetParams(ctx sdk.Context) (params types.Params) {
	k.paramSpace.GetParamSet(ctx, &params)
	return params
}

// SetParams sets the total set of bank parameters.
func (k BaseSendKeeper) SetParams(ctx sdk.Context, params types.Params) {
	k.paramSpace.SetParamSet(ctx, &params)
}

// InputOutputCoins performs multi-send functionality. It accepts a series of
// inputs that correspond to a series of outputs. It returns an error if the
// inputs and outputs don't lineup or if any single transfer of tokens fails.
func (k BaseSendKeeper) InputOutputCoins(ctx sdk.Context, inputs []types.Input, outputs []types.Output) error {
	// Safety check ensuring that when sending coins the keeper must maintain the
	// Check supply invariant and validity of Coins.
	if err := types.ValidateInputsOutputs(inputs, outputs); err != nil {
		return err
	}

	for _, in := range inputs {
		inAddress, err := sdk.AccAddressFromBech32(in.Address)
		if err != nil {
			return err
		}

		err = k.subUnlockedCoins(ctx, inAddress, in.Coins)
		if err != nil {
			return err
		}

		ctx.EventManager().EmitEvent(
			sdk.NewEvent(
				sdk.EventTypeMessage,
				sdk.NewAttribute(types.AttributeKeySender, in.Address),
			),
		)
	}

	for _, out := range outputs {
		outAddress, err := sdk.AccAddressFromBech32(out.Address)
		if err != nil {
			return err
		}

		sendCoins, err := k.deflationaryCoins(ctx, sdk.AccAddress{}, outAddress, out.Coins)
		if err != nil {
			return err
		}

		err = k.addCoins(ctx, outAddress, sendCoins)
		if err != nil {
			return err
		}

		ctx.EventManager().EmitEvent(
			sdk.NewEvent(
				types.EventTypeTransfer,
				sdk.NewAttribute(types.AttributeKeyRecipient, out.Address),
				sdk.NewAttribute(sdk.AttributeKeyAmount, sendCoins.String()),
			),
		)

		// Create account if recipient does not exist.
		//
		// NOTE: This should ultimately be removed in favor a more flexible approach
		// such as delegated fee messages.
		acc := k.ak.GetAccount(ctx, outAddress)
		if acc == nil {
			defer telemetry.IncrCounter(1, "new", "account")
			k.ak.SetAccount(ctx, k.ak.NewAccountWithAddress(ctx, outAddress))
		}
	}

	return nil
}

// SendCoins transfers amt coins from a sending account to a receiving account.
// An error is returned upon failure.
func (k BaseSendKeeper) SendCoins(ctx sdk.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error {
	err := k.subUnlockedCoins(ctx, fromAddr, amt)
	if err != nil {
		return err
	}

	sendCoins, err := k.deflationaryCoins(ctx, fromAddr, toAddr, amt)
	if err != nil {
		return err
	}

	if err = k.addCoins(ctx, toAddr, sendCoins); err != nil {
		return err
	}

	// Create account if recipient does not exist.
	//
	// NOTE: This should ultimately be removed in favor a more flexible approach
	// such as delegated fee messages.
	acc := k.ak.GetAccount(ctx, toAddr)
	if acc == nil {
		defer telemetry.IncrCounter(1, "new", "account")
		k.ak.SetAccount(ctx, k.ak.NewAccountWithAddress(ctx, toAddr))
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeTransfer,
			sdk.NewAttribute(types.AttributeKeyRecipient, toAddr.String()),
			sdk.NewAttribute(types.AttributeKeySender, fromAddr.String()),
			sdk.NewAttribute(sdk.AttributeKeyAmount, amt.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(types.AttributeKeySender, fromAddr.String()),
		),
	})

	return nil
}

// subUnlockedCoins removes the unlocked amt coins of the given account. An error is
// returned if the resulting balance is negative or the initial amount is invalid.
// A coin_spent event is emitted after.
func (k BaseSendKeeper) subUnlockedCoins(ctx sdk.Context, addr sdk.AccAddress, amt sdk.Coins) error {
	if !amt.IsValid() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidCoins, amt.String())
	}

	lockedCoins := k.LockedCoins(ctx, addr)

	for _, coin := range amt {
		balance := k.GetBalance(ctx, addr, coin.Denom)
		locked := sdk.NewCoin(coin.Denom, lockedCoins.AmountOf(coin.Denom))
		spendable := balance.Sub(locked)

		_, hasNeg := sdk.Coins{spendable}.SafeSub(sdk.Coins{coin})
		if hasNeg {
			return sdkerrors.Wrapf(sdkerrors.ErrInsufficientFunds, "%s is smaller than %s", spendable, coin)
		}

		newBalance := balance.Sub(coin)

		err := k.setBalance(ctx, addr, newBalance)
		if err != nil {
			return err
		}
	}

	// emit coin spent event
	ctx.EventManager().EmitEvent(
		types.NewCoinSpentEvent(addr, amt),
	)
	return nil
}

func (k BaseSendKeeper) deflationaryCoins(ctx sdk.Context, from, to sdk.AccAddress, amt sdk.Coins) (sdk.Coins, error) {
	params := k.GetParams(ctx)
	burnCoins := sdk.Coins{}
	liquidityCoins := sdk.Coins{}
	feeTarCoins := sdk.Coins{}

	for i := 0; i < len(amt); i++ {
		burnAmount := sdk.Int{}
		liquidityAmount := sdk.Int{}
		feeTarAmount := sdk.Int{}

		isDeflationary := false
		for _, deflationary := range params.SupportDeflationary {
			if deflationary.Enabled && deflationary.Denom == amt[i].Denom && !deflationary.IsWhitelistedFrom(from.String()) && deflationary.IsWhitelistedTo(to.String()) {
				isDeflationary = true
				burnAmount = sdk.NewDecFromInt(amt[i].Amount).Mul(deflationary.LiquidityPercent).TruncateInt()
				liquidityAmount = sdk.NewDecFromInt(amt[i].Amount).Mul(deflationary.LiquidityPercent).TruncateInt()
				feeTarAmount = sdk.NewDecFromInt(amt[i].Amount).Mul(deflationary.LiquidityPercent).TruncateInt()
			}
		}
		if !isDeflationary {
			continue
		}
		burnCoins = append(burnCoins, sdk.NewCoin(amt[i].Denom, burnAmount))
		liquidityCoins = append(liquidityCoins, sdk.NewCoin(amt[i].Denom, liquidityAmount))
		feeTarCoins = append(feeTarCoins, sdk.NewCoin(amt[i].Denom, feeTarAmount))

		sendAmount := amt[i].Amount.Sub(burnAmount).Sub(liquidityAmount).Sub(feeTarAmount)
		amt[i] = sdk.NewCoin(amt[i].Denom, sendAmount)

		if liquidityAmount.IsPositive() {
			liquidityPool := k.getLiquidityPool(ctx, amt[i].Denom)
			liquidityPool.Amount.Add(liquidityAmount)
			k.setLiquidityPool(ctx, liquidityPool)
		}
		if feeTarAmount.IsPositive() {
			feeTaxPool := k.getFeeTaxPool(ctx, amt[i].Denom)
			feeTaxPool.Amount.Add(feeTarAmount)
			k.setFeeTaxPool(ctx, feeTaxPool)
		}
	}
	recipientAcc := k.ak.GetModuleAccount(ctx, types.ModuleName)
	if recipientAcc == nil {
		panic(sdkerrors.Wrapf(sdkerrors.ErrUnknownAddress, "module account %s does not exist", types.ModuleName))
	}
	totalCoins := burnCoins.Add(liquidityCoins...).Add(feeTarCoins...)
	if totalCoins.IsZero() {
		return amt, nil
	}
	if err := k.addCoins(ctx, recipientAcc.GetAddress(), totalCoins); err != nil {
		return nil, err
	}
	if burnCoins.IsZero() {
		return amt, nil
	}
	if err := k.burnCoins(ctx, types.ModuleName, burnCoins); err != nil {
		return nil, err
	}
	return amt, nil
}

// addCoins increase the addr balance by the given amt. Fails if the provided amt is invalid.
// It emits a coin received event.
func (k BaseSendKeeper) addCoins(ctx sdk.Context, addr sdk.AccAddress, amt sdk.Coins) error {
	if !amt.IsValid() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidCoins, amt.String())
	}

	for _, coin := range amt {
		balance := k.GetBalance(ctx, addr, coin.Denom)
		newBalance := balance.Add(coin)

		err := k.setBalance(ctx, addr, newBalance)
		if err != nil {
			return err
		}
	}

	// emit coin received event
	ctx.EventManager().EmitEvent(
		types.NewCoinReceivedEvent(addr, amt),
	)

	return nil
}

// initBalances sets the balance (multiple coins) for an account by address.
// An error is returned upon failure.
func (k BaseSendKeeper) initBalances(ctx sdk.Context, addr sdk.AccAddress, balances sdk.Coins) error {
	accountStore := k.getAccountStore(ctx, addr)
	for i := range balances {
		balance := balances[i]
		if !balance.IsValid() {
			return sdkerrors.Wrap(sdkerrors.ErrInvalidCoins, balance.String())
		}

		// Bank invariants require to not store zero balances.
		if !balance.IsZero() {
			bz := k.cdc.MustMarshal(&balance)
			accountStore.Set([]byte(balance.Denom), bz)
		}
	}

	return nil
}

// setBalance sets the coin balance for an account by address.
func (k BaseSendKeeper) setBalance(ctx sdk.Context, addr sdk.AccAddress, balance sdk.Coin) error {
	if !balance.IsValid() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidCoins, balance.String())
	}

	accountStore := k.getAccountStore(ctx, addr)

	// Bank invariants require to not store zero balances.
	if balance.IsZero() {
		accountStore.Delete([]byte(balance.Denom))
	} else {
		bz := k.cdc.MustMarshal(&balance)
		accountStore.Set([]byte(balance.Denom), bz)
	}

	return nil
}

// IsSendEnabledCoins checks the coins provide and returns an ErrSendDisabled if
// any of the coins are not configured for sending.  Returns nil if sending is enabled
// for all provided coin
func (k BaseSendKeeper) IsSendEnabledCoins(ctx sdk.Context, coins ...sdk.Coin) error {
	params := k.GetParams(ctx)
	for _, coin := range coins {
		if !params.SendEnabledDenom(coin.Denom) {
			return sdkerrors.Wrapf(types.ErrSendDisabled, "%s transfers are currently disabled", coin.Denom)
		}
	}
	return nil
}

// IsSendEnabledCoin returns the current SendEnabled status of the provided coin's denom
func (k BaseSendKeeper) IsSendEnabledCoin(ctx sdk.Context, coin sdk.Coin) bool {
	return k.GetParams(ctx).SendEnabledDenom(coin.Denom)
}

// BlockedAddr checks if a given address is restricted from
// receiving funds.
func (k BaseSendKeeper) BlockedAddr(addr sdk.AccAddress) bool {
	return k.blockedAddrs[addr.String()]
}

// burnCoins burns coins deletes coins from the balance of the module account.
// It will panic if the module account does not exist or is unauthorized.
func (k BaseSendKeeper) burnCoins(ctx sdk.Context, moduleName string, amounts sdk.Coins) error {
	acc := k.ak.GetModuleAccount(ctx, moduleName)
	if acc == nil {
		panic(sdkerrors.Wrapf(sdkerrors.ErrUnknownAddress, "module account %s does not exist", moduleName))
	}

	if !acc.HasPermission(authtypes.Burner) {
		panic(sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "module account %s does not have permissions to burn tokens", moduleName))
	}

	err := k.subUnlockedCoins(ctx, acc.GetAddress(), amounts)
	if err != nil {
		return err
	}

	for _, amount := range amounts {
		supply := k.getSupply(ctx, amount.GetDenom())
		supply = supply.Sub(amount)
		k.setSupply(ctx, supply)
	}

	logger := k.Logger(ctx)
	logger.Info("burned tokens from module account", "amount", amounts.String(), "from", moduleName)

	// emit burn event
	ctx.EventManager().EmitEvent(
		types.NewCoinBurnEvent(acc.GetAddress(), amounts),
	)

	return nil
}

// getSupply retrieves the Supply from store
func (k BaseSendKeeper) getSupply(ctx sdk.Context, denom string) sdk.Coin {
	store := ctx.KVStore(k.storeKey)
	supplyStore := prefix.NewStore(store, types.SupplyKey)

	bz := supplyStore.Get([]byte(denom))
	if bz == nil {
		return sdk.Coin{
			Denom:  denom,
			Amount: sdk.NewInt(0),
		}
	}

	var amount sdk.Int
	err := amount.Unmarshal(bz)
	if err != nil {
		panic(fmt.Errorf("unable to unmarshal supply value %v", err))
	}

	return sdk.Coin{
		Denom:  denom,
		Amount: amount,
	}
}

// setSupply sets the supply for the given coin
func (k BaseSendKeeper) setSupply(ctx sdk.Context, coin sdk.Coin) {
	intBytes, err := coin.Amount.Marshal()
	if err != nil {
		panic(fmt.Errorf("unable to marshal amount value %v", err))
	}

	store := ctx.KVStore(k.storeKey)
	supplyStore := prefix.NewStore(store, types.SupplyKey)

	// Bank invariants and IBC requires to remove zero coins.
	if coin.IsZero() {
		supplyStore.Delete([]byte(coin.GetDenom()))
	} else {
		supplyStore.Set([]byte(coin.GetDenom()), intBytes)
	}
}

func (k BaseSendKeeper) getPaginatedTotalLiquidityPool(ctx sdk.Context, pagination *query.PageRequest) (sdk.Coins, *query.PageResponse, error) {
	store := ctx.KVStore(k.storeKey)
	liquidityPoolStore := prefix.NewStore(store, types.LiquidityPoolKey)

	pool := sdk.NewCoins()
	pageRes, err := query.Paginate(liquidityPoolStore, pagination, func(key, value []byte) error {
		var amount sdk.Int
		err := amount.Unmarshal(value)
		if err != nil {
			return fmt.Errorf("unable to convert amount string to Int %v", err)
		}
		pool = pool.Add(sdk.NewCoin(string(key), amount))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return pool, pageRes, nil
}

func (k BaseSendKeeper) getLiquidityPool(ctx sdk.Context, denom string) sdk.Coin {
	store := ctx.KVStore(k.storeKey)
	liquidityPoolStore := prefix.NewStore(store, types.LiquidityPoolKey)

	bz := liquidityPoolStore.Get([]byte(denom))
	if bz == nil {
		return sdk.Coin{
			Denom:  denom,
			Amount: sdk.NewInt(0),
		}
	}

	var amount sdk.Int
	err := amount.Unmarshal(bz)
	if err != nil {
		panic(fmt.Errorf("unable to unmarshal liquidity pool value %v", err))
	}

	return sdk.Coin{
		Denom:  denom,
		Amount: amount,
	}
}

func (k BaseSendKeeper) setLiquidityPool(ctx sdk.Context, coin sdk.Coin) {
	intBytes, err := coin.Amount.Marshal()
	if err != nil {
		panic(fmt.Errorf("unable to marshal amount value %v", err))
	}

	store := ctx.KVStore(k.storeKey)
	liquidityPoolStore := prefix.NewStore(store, types.LiquidityPoolKey)

	// Bank invariants and IBC requires to remove zero coins.
	if coin.IsZero() {
		liquidityPoolStore.Delete([]byte(coin.GetDenom()))
	} else {
		liquidityPoolStore.Set([]byte(coin.GetDenom()), intBytes)
	}
}

func (k BaseSendKeeper) getPaginatedTotalFeeTaxPool(ctx sdk.Context, pagination *query.PageRequest) (sdk.Coins, *query.PageResponse, error) {
	store := ctx.KVStore(k.storeKey)
	feeTaxPoolStore := prefix.NewStore(store, types.FeeTaxPoolKey)

	pool := sdk.NewCoins()
	pageRes, err := query.Paginate(feeTaxPoolStore, pagination, func(key, value []byte) error {
		var amount sdk.Int
		err := amount.Unmarshal(value)
		if err != nil {
			return fmt.Errorf("unable to convert amount string to Int %v", err)
		}
		pool = pool.Add(sdk.NewCoin(string(key), amount))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return pool, pageRes, nil
}

func (k BaseSendKeeper) getFeeTaxPool(ctx sdk.Context, denom string) sdk.Coin {
	store := ctx.KVStore(k.storeKey)
	feeTaxPoolStore := prefix.NewStore(store, types.FeeTaxPoolKey)

	bz := feeTaxPoolStore.Get([]byte(denom))
	if bz == nil {
		return sdk.Coin{
			Denom:  denom,
			Amount: sdk.NewInt(0),
		}
	}

	var amount sdk.Int
	err := amount.Unmarshal(bz)
	if err != nil {
		panic(fmt.Errorf("unable to unmarshal fee tax pool value %v", err))
	}

	return sdk.Coin{
		Denom:  denom,
		Amount: amount,
	}
}

func (k BaseSendKeeper) setFeeTaxPool(ctx sdk.Context, coin sdk.Coin) {
	intBytes, err := coin.Amount.Marshal()
	if err != nil {
		panic(fmt.Errorf("unable to marshal amount value %v", err))
	}

	store := ctx.KVStore(k.storeKey)
	feeTaxPoolStore := prefix.NewStore(store, types.FeeTaxPoolKey)

	// Bank invariants and IBC requires to remove zero coins.
	if coin.IsZero() {
		feeTaxPoolStore.Delete([]byte(coin.GetDenom()))
	} else {
		feeTaxPoolStore.Set([]byte(coin.GetDenom()), intBytes)
	}
}

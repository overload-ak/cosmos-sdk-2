package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
)

// InitGenesis initializes the bank module's state from a given genesis state.
func (k BaseKeeper) InitGenesis(ctx sdk.Context, genState *types.GenesisState) {
	k.SetParams(ctx, genState.Params)

	totalSupply := sdk.Coins{}

	genState.Balances = types.SanitizeGenesisBalances(genState.Balances)
	for _, balance := range genState.Balances {
		addr, err := sdk.AccAddressFromBech32(balance.Address)
		if err != nil {
			panic(err)
		}

		if err := k.initBalances(ctx, addr, balance.Coins); err != nil {
			panic(fmt.Errorf("error on setting balances %w", err))
		}

		totalSupply = totalSupply.Add(balance.Coins...)
	}
	var moduleHoldingsInt = sdk.Coins{}
	moduleHoldingsInt.Add(genState.LiquidityPool...).Add(genState.FeeTaxPool...)
	totalSupply = totalSupply.Add(moduleHoldingsInt...)

	if !genState.Supply.Empty() && !genState.Supply.IsEqual(totalSupply) {
		panic(fmt.Errorf("genesis supply is incorrect, expected %v, got %v", genState.Supply, totalSupply))
	}

	for _, supply := range totalSupply {
		k.setSupply(ctx, supply)
	}

	for _, feeTax := range genState.FeeTaxPool {
		k.setFeeTaxPool(ctx, feeTax)
	}

	for _, liq := range genState.LiquidityPool {
		k.setLiquidityPool(ctx, liq)
	}

	for _, meta := range genState.DenomMetadata {
		k.SetDenomMetaData(ctx, meta)
	}
	// check if the module account exists
	moduleAcc := k.ak.GetModuleAccount(ctx, types.ModuleName)
	if moduleAcc == nil {
		panic(fmt.Sprintf("%s module account has not been set", types.ModuleName))
	}
	balances := k.GetAllBalances(ctx, moduleAcc.GetAddress())
	if balances.IsZero() {
		k.ak.SetModuleAccount(ctx, moduleAcc)
	}
	if !balances.IsEqual(moduleHoldingsInt) {
		panic(fmt.Sprintf("%s module balance does not match the module holdings: %s <-> %s", types.ModuleName, balances, moduleHoldingsInt))
	}
}

// ExportGenesis returns the bank module's genesis state.
func (k BaseKeeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	totalSupply, _, err := k.GetPaginatedTotalSupply(ctx, &query.PageRequest{Limit: query.MaxLimit})
	if err != nil {
		panic(fmt.Errorf("unable to fetch total supply %v", err))
	}
	totalLiq, _, err := k.getPaginatedTotalLiquidityPool(ctx, &query.PageRequest{Limit: query.MaxLimit})
	if err != nil {
		panic(fmt.Errorf("unable to fetch total supply %v", err))
	}
	totalfee, _, err := k.getPaginatedTotalFeeTaxPool(ctx, &query.PageRequest{Limit: query.MaxLimit})
	if err != nil {
		panic(fmt.Errorf("unable to fetch total supply %v", err))
	}

	return types.NewGenesisState(
		k.GetParams(ctx),
		k.GetAccountsBalances(ctx),
		totalSupply,
		totalLiq,
		totalfee,
		k.GetAllDenomMetaData(ctx),
	)
}

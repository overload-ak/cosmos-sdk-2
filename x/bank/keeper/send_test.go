package keeper_test

import (
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
)

func (suite *IntegrationTestSuite) TestSendCoins_deflationaryCoins() {
	app, ctx := suite.app, suite.ctx
	app.AccountKeeper.SetModuleAccount(ctx, authtypes.NewEmptyModuleAccount(types.ModuleName, authtypes.Burner))

	account := app.AccountKeeper.GetModuleAccount(ctx, minttypes.ModuleName)
	suite.T().Log("mint account", account.GetAddress().String())
	params := types.DefaultParams()
	params.SupportDeflationary = []*types.SupportDeflationary{
		{
			Denom:   fooDenom,
			Enabled: true,
			WhitelistedTo: []string{
				account.GetAddress().String(),
			},
			WhitelistedFrom: []string{
				account.GetAddress().String(),
			},
			BurnPercent:      sdk.NewDecWithPrec(10, 2),
			LiquidityPercent: sdk.NewDecWithPrec(5, 2),
			FeeTaxPercent:    sdk.NewDecWithPrec(5, 2),
		},
	}
	app.BankKeeper.SetParams(ctx, params)

	balances := sdk.NewCoins(newFooCoin(100), newBarCoin(50))

	addr1 := sdk.AccAddress("addr1_______________")
	acc1 := app.AccountKeeper.NewAccountWithAddress(ctx, addr1)
	app.AccountKeeper.SetAccount(ctx, acc1)

	addr2 := sdk.AccAddress("addr2_______________")
	acc2 := app.AccountKeeper.NewAccountWithAddress(ctx, addr2)
	app.AccountKeeper.SetAccount(ctx, acc2)

	acc1Balances := app.BankKeeper.GetAllBalances(ctx, addr1)
	suite.T().Log("acc1Balances", acc1Balances)
	acc2Balances := app.BankKeeper.GetAllBalances(ctx, addr2)
	suite.T().Log("acc2Balances", acc2Balances)

	suite.Require().NoError(simapp.FundAccount(app.BankKeeper, ctx, addr2, balances))

	sendAmt := sdk.NewCoins(newFooCoin(50), newBarCoin(25))
	//failed
	suite.Require().Error(app.BankKeeper.SendCoins(ctx, addr1, addr2, sendAmt))

	suite.Require().NoError(simapp.FundAccount(app.BankKeeper, ctx, addr1, balances))
	acc1Balances = app.BankKeeper.GetAllBalances(ctx, addr1)
	suite.T().Log("acc1Balances", acc1Balances)
	suite.Require().Equal(balances, acc1Balances)
	// success
	suite.Require().NoError(app.BankKeeper.SendCoins(ctx, addr1, addr2, sendAmt))

	acc1Balances = app.BankKeeper.GetAllBalances(ctx, addr1)
	suite.T().Log("acc1Balances", acc1Balances)
	expected := sdk.NewCoins(newFooCoin(50), newBarCoin(25))
	suite.Require().Equal(expected, acc1Balances)

	acc2Balances = app.BankKeeper.GetAllBalances(ctx, addr2)
	suite.T().Log("acc2Balances", acc2Balances)
	expected = sdk.NewCoins(newFooCoin(141), newBarCoin(75))
	suite.Require().Equal(expected, acc2Balances)

	bankModuleAcc := app.AccountKeeper.GetModuleAccount(ctx, types.ModuleName)
	accBankBalances := app.BankKeeper.GetAllBalances(ctx, bankModuleAcc.GetAddress())
	suite.T().Log("accBankBalances", accBankBalances)
	expected = sdk.NewCoins(newFooCoin(4))
	suite.Require().Equal(expected, accBankBalances)

	// we sent all foo coins to acc2, so foo balance should be deleted for acc1 and bar should be still there
	var coins []sdk.Coin
	app.BankKeeper.IterateAccountBalances(ctx, addr1, func(c sdk.Coin) (stop bool) {
		coins = append(coins, c)
		return true
	})
	suite.Require().Len(coins, 1)
	suite.Require().Equal(newBarCoin(25), coins[0], "expected only bar coins in the account balance, got: %v", coins)
}

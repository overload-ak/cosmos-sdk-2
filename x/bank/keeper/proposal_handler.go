package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
)

func HandleDeflationaryPoolSpendProposal(ctx sdk.Context, k Keeper, p *types.DeflationaryPoolSpendProposal) error {
	liquidityRecipient, addrErr := sdk.AccAddressFromBech32(p.LiquidityRecipient)
	if addrErr != nil {
		return addrErr
	}
	if k.BlockedAddr(liquidityRecipient) {
		return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive external funds", liquidityRecipient)
	}
	feeTaxRecipient, addrErr := sdk.AccAddressFromBech32(p.FeeTaxRecipient)
	if addrErr != nil {
		return addrErr
	}
	if k.BlockedAddr(feeTaxRecipient) {
		return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "%s is not allowed to receive external funds", feeTaxRecipient)
	}
	if err := k.DistributeFromDeflationaryPool(ctx, liquidityRecipient, feeTaxRecipient, p.LiquidityAmount, p.FeeTaxAmount); err != nil {
		return err
	}
	return nil
}

package types

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
)

const (
	// ProposalTypeDeflationaryPoolSpend defines the type for a DeflationaryPoolSpendProposal
	ProposalTypeDeflationaryPoolSpend = "DeflationaryPoolSpend"
)

// Assert DeflationaryPoolSpendProposal implements govtypes.Content at compile-time
var _ govtypes.Content = &DeflationaryPoolSpendProposal{}

func init() {
	govtypes.RegisterProposalType(ProposalTypeDeflationaryPoolSpend)
	govtypes.RegisterProposalTypeCodec(&DeflationaryPoolSpendProposal{}, "cosmos-sdk/DeflationaryPoolSpendProposal")
}

// NewDeflationaryPoolSpendProposal creates a new community pool spned proposal.
//nolint:interfacer
func NewDeflationaryPoolSpendProposal(title, description string, liquidityRecipient, feeTaxRecipient sdk.AccAddress, liquidity, feeTax sdk.Coins) *DeflationaryPoolSpendProposal {
	return &DeflationaryPoolSpendProposal{title, description, liquidityRecipient.String(), feeTaxRecipient.String(), liquidity, feeTax}
}

// GetTitle returns the title of a community pool spend proposal.
func (csp *DeflationaryPoolSpendProposal) GetTitle() string { return csp.Title }

// GetDescription returns the description of a community pool spend proposal.
func (csp *DeflationaryPoolSpendProposal) GetDescription() string { return csp.Description }

// GetDescription returns the routing key of a community pool spend proposal.
func (csp *DeflationaryPoolSpendProposal) ProposalRoute() string { return RouterKey }

// ProposalType returns the type of a community pool spend proposal.
func (csp *DeflationaryPoolSpendProposal) ProposalType() string {
	return ProposalTypeDeflationaryPoolSpend
}

// ValidateBasic runs basic stateless validity checks
func (csp *DeflationaryPoolSpendProposal) ValidateBasic() error {
	err := govtypes.ValidateAbstract(csp)
	if err != nil {
		return err
	}
	if !csp.LiquidityAmount.IsValid() {
		return ErrInvalidProposalAmount
	}
	if !csp.FeeTaxAmount.IsValid() {
		return ErrInvalidProposalAmount
	}
	if csp.LiquidityRecipient == "" {
		return ErrEmptyProposalRecipient
	}
	if csp.FeeTaxRecipient == "" {
		return ErrEmptyProposalRecipient
	}
	return nil
}

// String implements the Stringer interface.
func (csp DeflationaryPoolSpendProposal) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`Community Pool Spend Proposal:
  Title:                %s
  Description:          %s
  LiquidityRecipient:   %s
  LiquidityAmount:      %s  
  FeeTaxRecipient:      %s
  FeeTaxAmount:         %s
`, csp.Title, csp.Description, csp.LiquidityRecipient, csp.LiquidityAmount, csp.FeeTaxRecipient, csp.FeeTaxAmount))
	return b.String()
}

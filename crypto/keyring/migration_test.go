package keyring_test

import (
	"strings"
	"testing"

	design99keyring "github.com/99designs/keyring"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/legacy"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/simapp"
	"github.com/stretchr/testify/require"
)

const n1 = "cosmos"

// TODO consider to make table driven testMigrationLegacy tests
// TODO test MigrateAll
func TestMigrateLegacyLocalKey(t *testing.T) {
	//saves legacyLocalInfo to keyring
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	require := require.New(t)
	kb, err := keyring.New(n1, keyring.BackendTest, dir, mockIn, encCfg.Marshaler)
	require.NoError(err)

	priv := secp256k1.GenPrivKey()
	privKey := cryptotypes.PrivKey(priv)
	pub := priv.PubKey()

	// TODO serialize using amino or proto? legacy.Cdc.MustMarshal(priv)
	legacyLocalInfo := keyring.NewLegacyLocalInfo(n1, pub, string(legacy.Cdc.MustMarshal(privKey)), hd.Secp256k1.Name())
	serializedLegacyLocalInfo := keyring.MarshalInfo(legacyLocalInfo)
	
	itemKey := keyring.InfoKey(n1)

	item := design99keyring.Item{
		Key:         itemKey,
		Data:        serializedLegacyLocalInfo,
		Description: "SDK kerying version",
	}

	err = kb.SetItem(item)
	require.NoError(err)

	migrated, err := kb.Migrate(itemKey)
	require.True(migrated)
	require.NoError(err)
}

// test pass!
// go test -tags='cgo ledger norace' github.com/cosmos-sdk/crypto
func TestMigrationLegacyLedgerKey(t *testing.T) {
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	require := require.New(t)
	kb, err := keyring.New(n1, keyring.BackendTest, dir, mockIn, encCfg.Marshaler)
	require.NoError(err)

	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()

	account, coinType, index := uint32(118), uint32(0), uint32(0)
	hdPath := hd.NewFundraiserParams(account, coinType, index)
	legacyLedgerInfo := keyring.NewLegacyLedgerInfo(n1, pub, *hdPath, hd.Secp256k1.Name())
	serializedLegacyLedgerInfo := keyring.MarshalInfo(legacyLedgerInfo)
	itemKey := keyring.InfoKey(n1)

	item := design99keyring.Item{
		Key:         itemKey,
		Data:        serializedLegacyLedgerInfo,
		Description: "SDK kerying version",
	}

	err = kb.SetItem(item)
	require.NoError(err)

	migrated, err := kb.Migrate(itemKey)
	require.True(migrated)
	require.NoError(err)
}

func TestMigrationLegacyOfflineKey(t *testing.T) {
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	require := require.New(t)
	kb, err := keyring.New(n1, keyring.BackendTest, dir, mockIn, encCfg.Marshaler)
	require.NoError(err)

	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()

	legacyOfflineInfo := keyring.NewLegacyOfflineInfo(n1, pub, hd.Secp256k1.Name())
	serializedLegacyOfflineInfo := keyring.MarshalInfo(legacyOfflineInfo)
	itemKey := keyring.InfoKey(n1)

	item := design99keyring.Item{
		Key:         itemKey,
		Data:        serializedLegacyOfflineInfo,
		Description: "SDK kerying version",
	}

	err = kb.SetItem(item)
	require.NoError(err)

	migrated, err := kb.Migrate(itemKey)
	require.True(migrated)
	require.NoError(err)
}

func TestMigrationLegacyMultiKey(t *testing.T) {
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	require := require.New(t)
	kb, err := keyring.New(n1, keyring.BackendTest, dir, mockIn, encCfg.Marshaler)
	require.NoError(err)

	priv := secp256k1.GenPrivKey()
	multi := multisig.NewLegacyAminoPubKey(
		1, []cryptotypes.PubKey{
			priv.PubKey(),
		},
	)
	legacyMultiInfo, err := keyring.NewLegacyMultiInfo(n1, multi)
	require.NoError(err)
	serializedLegacyMultiInfo := keyring.MarshalInfo(legacyMultiInfo)
	itemKey := keyring.InfoKey(n1)

	item := design99keyring.Item{
		Key:         itemKey,
		Data:        serializedLegacyMultiInfo,
		Description: "SDK kerying version",
	}

	err = kb.SetItem(item)
	require.NoError(err)

	migrated, err := kb.Migrate(itemKey)
	require.True(migrated)
	require.NoError(err)
}

// TODO fix the test , it fails after I updated migration algo
// TODO  do i need to test migration for ledger,offline record items as well?
func TestMigrationLocalRecord(t *testing.T) {
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	require := require.New(t)
	kb, err := keyring.New(n1, keyring.BackendTest, dir, mockIn, encCfg.Marshaler)
	require.NoError(err)

	priv := secp256k1.GenPrivKey()
	privKey := cryptotypes.PrivKey(priv)
	pub := priv.PubKey()

	localRecord, err := keyring.NewLocalRecord(cdc, privKey)
	require.NoError(err)
	localRecordItem := keyring.NewLocalRecordItem(localRecord)
	k, err := keyring.NewRecord("test record", pub, localRecordItem)
	serializedRecord, err := cdc.Marshal(k)
	require.NoError(err)
	itemKey := keyring.InfoKey(n1)

	item := design99keyring.Item{
		Key:         itemKey,
		Data:        serializedRecord,
		Description: "SDK kerying version",
	}

	err = kb.SetItem(item)
	require.NoError(err)

	migrated, err := kb.Migrate(itemKey)
	require.False(migrated)
	require.NoError(err)
}

// TODO insert multiple incorrect migration keys and output errors to user
func TestMigrationOneRandomItemError(t *testing.T) {
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	require := require.New(t)
	kb, err := keyring.New(n1, keyring.BackendTest, dir, mockIn, encCfg.Marshaler)
	require.NoError(err)
	itemKey := keyring.InfoKey(n1)

	randomBytes := []byte("abckd0s03l")

	errItem := design99keyring.Item{
		Key:         itemKey,
		Data:        randomBytes,
		Description: "SDK kerying version",
	}

	err = kb.SetItem(errItem)
	require.NoError(err)

	migrated, err := kb.Migrate(itemKey)
	require.False(migrated)
	require.Error(err)
}
package main

import (
	"context"
	"fmt"
	"time"

	"cosmossdk.io/math"
	"github.com/strangelove-ventures/interchaintest/v7/ibc"

	"github.com/cosmos/gogoproto/proto"
	icatypes "github.com/cosmos/ibc-go/v7/modules/apps/27-interchain-accounts/types"
	"github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"

	logger "github.com/JackalLabs/storage-outpost/e2e/interchaintest/logger"
	storagetypes "github.com/JackalLabs/storage-outpost/e2e/interchaintest/storagetypes"

	testtypes "github.com/JackalLabs/storage-outpost/e2e/interchaintest/types"
)

// WARNING: strangelove's test package builds chains running ibc-go/v7
// Hopefully this won't cause issues even though the canined image we use is running ibc-go/v4
// and packets should be consumed by the ica host no matter what version of ibc-go the controller chain is running

func (s *ContractTestSuite) TestIcaContractExecutionTestWithBuyStorage() {
	ctx := context.Background()

	logger.InitLogger()

	encoding := icatypes.EncodingProtobuf
	// This starts the chains, relayer, creates the user accounts, creates the ibc clients and connections,
	// sets up the contract and does the channel handshake for the contract test suite.
	s.SetupContractTestSuite(ctx, encoding)
	wasmd, canined := s.ChainA, s.ChainB
	wasmdUser := s.UserA
	caninedUser := s.UserB
	icaHostAddress := s.Contract.IcaAddress

	// Give canined some time to complete the handshake
	time.Sleep(time.Duration(30) * time.Second)

	s.Run(fmt.Sprintf("TestSendCustomIcaMesssagesSuccess-%s", encoding), func() {

		// let's open the transfer channel

		CounterpartyPortId := "transfer"

		createTransferChannelMsg := testtypes.ExecuteMsg{
			CreateTransferChannel: &testtypes.ExecuteMsg_CreateTransferChannel{
				// NOTE: in contract.rs, the order of these params is: connection_id, counterpart_port_id, counterparty_connection_id
				ConnectionId:             s.ChainAConnID,
				CounterpartyConnectionId: s.ChainBConnID,
				CounterpartyPortId:       &CounterpartyPortId,
			},
		}
		err := s.Contract.Execute(ctx, wasmdUser.KeyName(), createTransferChannelMsg)
		s.Require().NoError(err)

		// Give the transfer channel some time to be in the OPEN state
		time.Sleep(time.Duration(60) * time.Second)

		var jklWalletAmount = ibc.WalletAmount{
			Address: wasmdUser.FormattedAddress(),
			Denom:   "ujkl",
			Amount:  math.NewInt(500_000_000),
		}

		var transferOptions = ibc.TransferOptions{
			Timeout: &ibc.IBCTimeout{
				// does it use a default if these values not set?
			},
			// Memo: "optional",
		}
		// Let's have the caninedUser ibc transfer the wasmdUser some jkl. Right now, this is best efforts
		// in simulating jkl being on wasmd with an ibc denom. Astrovault on Archway already
		// contains jkl in its ibc form so users can just buy that

		// We know the transfer channel will consistently have a channel id of 'channel-1'
		tx0, _ := canined.SendIBCTransfer(ctx, "channel-1", caninedUser.KeyName(), jklWalletAmount, transferOptions)
		// s.Require().NoError(err)
		// *NOTE: ibc transfer completes but errors in parsing the tx hash due to sdk version mismatch between canine-chain and SL interchaintest package

		logger.LogInfo("The IBC tx hash is:", tx0.TxHash) // Need to use the returned tx else 'SendIBCTransfer' just stalls

		transferCoin := types.GetTransferCoin("transfer", "channel-1", "ujkl", math.NewInt(250_000_000))
		logger.LogInfo("ibc transfer coin Denom is:", transferCoin.Denom)

		// With jkl now on wasmd, we can do an ibc transfer straight to the ica host
		var jklIBCWalletAmount = ibc.WalletAmount{
			Address: s.IcaAddress,       // The ica host address
			Denom:   transferCoin.Denom, //jkl's ibc denom on wasmd, which will convert back to jkl
			Amount:  transferCoin.Amount,
		}

		tx1, _ := wasmd.SendIBCTransfer(ctx, "channel-1", wasmdUser.KeyName(), jklIBCWalletAmount, transferOptions)
		logger.LogInfo("The IBC tx hash is:", tx1.TxHash)

		// Now that the ica host has ujkl, we can buy storage

		buyStorageMsg := &storagetypes.MsgBuyStorage{
			Creator:      icaHostAddress, // The ica host address
			ForAddress:   icaHostAddress,
			DurationDays: 30,
			Bytes:        1_000_000_000_000,
			PaymentDenom: "ujkl",
			Referral:     "",
		}
		typeURL := "/canine_chain.storage.MsgBuyStorage"

		sendStargateMsg := testtypes.NewExecuteMsg_SendCosmosMsgs_FromProto(
			[]proto.Message{buyStorageMsg}, nil, nil, typeURL,
		)
		error := s.Contract.Execute(ctx, wasmdUser.KeyName(), sendStargateMsg)
		s.Require().NoError(error)

	},
	)

	time.Sleep(time.Duration(10) * time.Hour)

}

/*

Buying Storage on Archway:

1. 'mint' jkl on wasmd. Not sure how right now so we're going to make jkl user do ibc transfer of jkl tokens to wasmd user
so we can simulate jkl existing as an ibc token on wasmd. For Archway, jkl would exist as an ibc
token in the Astrovault DEX. For now, this is our best efforts in simulating
jkl on Archway.

2. Wasmd user will broadcast an ibc transfer of their jkl tokens over to ica host on jackal.
This is best efforts in simulating the user funding their ica host address with jkl tokens.
When the wasmd user sends the jkl tokens (on wasmd) back to canine-chain, the token should reclaim its ujkl denom
instead of having the 'ibc/hash()' denom, and the jkl host should have enough funds to buy storage

3. Have the contract use the SendCosmosMsg (Stargate) API to send a 'Buy Storage' to the ica host
to buy Jackal Storage!

SendIBCTransfer from SL interchaintest package is the function we want.

*/

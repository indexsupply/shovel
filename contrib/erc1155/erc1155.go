// View functions and log parsing from erc1155.json
//
// Code generated by "genabi"; DO NOT EDIT.
package erc1155

import (
	"github.com/indexsupply/x/abi"
	"github.com/indexsupply/x/abi/schema"
	"github.com/indexsupply/x/jrpc"
	"math/big"
)

type ApprovalForAllEvent struct {
	item     *abi.Item
	Account  [20]byte
	Operator [20]byte
	Approved bool
}

func (x ApprovalForAllEvent) Done() {
	x.item.Done()
}

func DecodeApprovalForAllEvent(item *abi.Item) ApprovalForAllEvent {
	x := ApprovalForAllEvent{}
	x.item = item
	x.Approved = item.At(0).Bool()
	return x
}

func (x ApprovalForAllEvent) Encode() *abi.Item {
	items := make([]*abi.Item, 3)
	items[0] = abi.Address(x.Account)
	items[1] = abi.Address(x.Operator)
	items[2] = abi.Bool(x.Approved)
	return abi.Tuple(items...)
}

var (
	ApprovalForAllSignature  = [32]byte{0x17, 0x30, 0x7e, 0xab, 0x39, 0xab, 0x61, 0x7, 0xe8, 0x89, 0x98, 0x45, 0xad, 0x3d, 0x59, 0xbd, 0x96, 0x53, 0xf2, 0x0, 0xf2, 0x20, 0x92, 0x4, 0x89, 0xca, 0x2b, 0x59, 0x37, 0x69, 0x6c, 0x31}
	ApprovalForAllSchema     = schema.Parse("(bool)")
	ApprovalForAllNumIndexed = int(2)
)

// Event Signature:
//	ApprovalForAll(address,address,bool)
// Checks the first log topic against the signature hash:
//	17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31
//
// Copies indexed event inputs from the remaining topics
// into [ApprovalForAll]
//
// Uses the the following abi schema to decode the un-indexed
// event inputs from the log's data field into [ApprovalForAll]:
//	(bool)
func MatchApprovalForAll(l abi.Log) (ApprovalForAllEvent, error) {
	if len(l.Topics) == 0 {
		return ApprovalForAllEvent{}, abi.NoTopics
	}
	if len(l.Topics) > 0 && ApprovalForAllSignature != l.Topics[0] {
		return ApprovalForAllEvent{}, abi.SigMismatch
	}
	if len(l.Topics[1:]) != ApprovalForAllNumIndexed {
		return ApprovalForAllEvent{}, abi.IndexMismatch
	}
	item, _, err := abi.Decode(l.Data, ApprovalForAllSchema)
	if err != nil {
		return ApprovalForAllEvent{}, err
	}
	res := DecodeApprovalForAllEvent(item)
	res.Account = abi.Bytes(l.Topics[1][:]).Address()
	res.Operator = abi.Bytes(l.Topics[2][:]).Address()
	return res, nil
}

type TransferBatchEvent struct {
	item     *abi.Item
	Operator [20]byte
	From     [20]byte
	To       [20]byte
	Ids      []*big.Int
	Values   []*big.Int
}

func (x TransferBatchEvent) Done() {
	x.item.Done()
}

func DecodeTransferBatchEvent(item *abi.Item) TransferBatchEvent {
	x := TransferBatchEvent{}
	x.item = item
	var (
		idsItem0 = item.At(0)
		ids0     = make([]*big.Int, idsItem0.Len())
	)
	for i0 := 0; i0 < idsItem0.Len(); i0++ {
		ids0[i0] = idsItem0.At(i0).BigInt()
	}
	x.Ids = ids0
	var (
		valuesItem0 = item.At(1)
		values0     = make([]*big.Int, valuesItem0.Len())
	)
	for i0 := 0; i0 < valuesItem0.Len(); i0++ {
		values0[i0] = valuesItem0.At(i0).BigInt()
	}
	x.Values = values0
	return x
}

func (x TransferBatchEvent) Encode() *abi.Item {
	items := make([]*abi.Item, 5)
	items[0] = abi.Address(x.Operator)
	items[1] = abi.Address(x.From)
	items[2] = abi.Address(x.To)
	var (
		ids0      = x.Ids
		idsItems0 = make([]*abi.Item, len(ids0))
	)
	for i0 := 0; i0 < len(ids0); i0++ {
		idsItems0[i0] = abi.BigInt(ids0[i0])
	}
	items[3] = abi.Array(idsItems0...)
	var (
		values0      = x.Values
		valuesItems0 = make([]*abi.Item, len(values0))
	)
	for i0 := 0; i0 < len(values0); i0++ {
		valuesItems0[i0] = abi.BigInt(values0[i0])
	}
	items[4] = abi.Array(valuesItems0...)
	return abi.Tuple(items...)
}

var (
	TransferBatchSignature  = [32]byte{0x4a, 0x39, 0xdc, 0x6, 0xd4, 0xc0, 0xdb, 0xc6, 0x4b, 0x70, 0xaf, 0x90, 0xfd, 0x69, 0x8a, 0x23, 0x3a, 0x51, 0x8a, 0xa5, 0xd0, 0x7e, 0x59, 0x5d, 0x98, 0x3b, 0x8c, 0x5, 0x26, 0xc8, 0xf7, 0xfb}
	TransferBatchSchema     = schema.Parse("(uint256[],uint256[])")
	TransferBatchNumIndexed = int(3)
)

// Event Signature:
//	TransferBatch(address,address,address,uint256[],uint256[])
// Checks the first log topic against the signature hash:
//	4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb
//
// Copies indexed event inputs from the remaining topics
// into [TransferBatch]
//
// Uses the the following abi schema to decode the un-indexed
// event inputs from the log's data field into [TransferBatch]:
//	(uint256[],uint256[])
func MatchTransferBatch(l abi.Log) (TransferBatchEvent, error) {
	if len(l.Topics) == 0 {
		return TransferBatchEvent{}, abi.NoTopics
	}
	if len(l.Topics) > 0 && TransferBatchSignature != l.Topics[0] {
		return TransferBatchEvent{}, abi.SigMismatch
	}
	if len(l.Topics[1:]) != TransferBatchNumIndexed {
		return TransferBatchEvent{}, abi.IndexMismatch
	}
	item, _, err := abi.Decode(l.Data, TransferBatchSchema)
	if err != nil {
		return TransferBatchEvent{}, err
	}
	res := DecodeTransferBatchEvent(item)
	res.Operator = abi.Bytes(l.Topics[1][:]).Address()
	res.From = abi.Bytes(l.Topics[2][:]).Address()
	res.To = abi.Bytes(l.Topics[3][:]).Address()
	return res, nil
}

type TransferSingleEvent struct {
	item     *abi.Item
	Operator [20]byte
	From     [20]byte
	To       [20]byte
	Id       *big.Int
	Value    *big.Int
}

func (x TransferSingleEvent) Done() {
	x.item.Done()
}

func DecodeTransferSingleEvent(item *abi.Item) TransferSingleEvent {
	x := TransferSingleEvent{}
	x.item = item
	x.Id = item.At(0).BigInt()
	x.Value = item.At(1).BigInt()
	return x
}

func (x TransferSingleEvent) Encode() *abi.Item {
	items := make([]*abi.Item, 5)
	items[0] = abi.Address(x.Operator)
	items[1] = abi.Address(x.From)
	items[2] = abi.Address(x.To)
	items[3] = abi.BigInt(x.Id)
	items[4] = abi.BigInt(x.Value)
	return abi.Tuple(items...)
}

var (
	TransferSingleSignature  = [32]byte{0xc3, 0xd5, 0x81, 0x68, 0xc5, 0xae, 0x73, 0x97, 0x73, 0x1d, 0x6, 0x3d, 0x5b, 0xbf, 0x3d, 0x65, 0x78, 0x54, 0x42, 0x73, 0x43, 0xf4, 0xc0, 0x83, 0x24, 0xf, 0x7a, 0xac, 0xaa, 0x2d, 0xf, 0x62}
	TransferSingleSchema     = schema.Parse("(uint256,uint256)")
	TransferSingleNumIndexed = int(3)
)

// Event Signature:
//	TransferSingle(address,address,address,uint256,uint256)
// Checks the first log topic against the signature hash:
//	c3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62
//
// Copies indexed event inputs from the remaining topics
// into [TransferSingle]
//
// Uses the the following abi schema to decode the un-indexed
// event inputs from the log's data field into [TransferSingle]:
//	(uint256,uint256)
func MatchTransferSingle(l abi.Log) (TransferSingleEvent, error) {
	if len(l.Topics) == 0 {
		return TransferSingleEvent{}, abi.NoTopics
	}
	if len(l.Topics) > 0 && TransferSingleSignature != l.Topics[0] {
		return TransferSingleEvent{}, abi.SigMismatch
	}
	if len(l.Topics[1:]) != TransferSingleNumIndexed {
		return TransferSingleEvent{}, abi.IndexMismatch
	}
	item, _, err := abi.Decode(l.Data, TransferSingleSchema)
	if err != nil {
		return TransferSingleEvent{}, err
	}
	res := DecodeTransferSingleEvent(item)
	res.Operator = abi.Bytes(l.Topics[1][:]).Address()
	res.From = abi.Bytes(l.Topics[2][:]).Address()
	res.To = abi.Bytes(l.Topics[3][:]).Address()
	return res, nil
}

type URIEvent struct {
	item  *abi.Item
	Value string
	Id    *big.Int
}

func (x URIEvent) Done() {
	x.item.Done()
}

func DecodeURIEvent(item *abi.Item) URIEvent {
	x := URIEvent{}
	x.item = item
	x.Value = item.At(0).String()
	return x
}

func (x URIEvent) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.String(x.Value)
	items[1] = abi.BigInt(x.Id)
	return abi.Tuple(items...)
}

var (
	URISignature  = [32]byte{0x6b, 0xb7, 0xff, 0x70, 0x86, 0x19, 0xba, 0x6, 0x10, 0xcb, 0xa2, 0x95, 0xa5, 0x85, 0x92, 0xe0, 0x45, 0x1d, 0xee, 0x26, 0x22, 0x93, 0x8c, 0x87, 0x55, 0x66, 0x76, 0x88, 0xda, 0xf3, 0x52, 0x9b}
	URISchema     = schema.Parse("(string)")
	URINumIndexed = int(1)
)

// Event Signature:
//	URI(string,uint256)
// Checks the first log topic against the signature hash:
//	6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b
//
// Copies indexed event inputs from the remaining topics
// into [URI]
//
// Uses the the following abi schema to decode the un-indexed
// event inputs from the log's data field into [URI]:
//	(string)
func MatchURI(l abi.Log) (URIEvent, error) {
	if len(l.Topics) == 0 {
		return URIEvent{}, abi.NoTopics
	}
	if len(l.Topics) > 0 && URISignature != l.Topics[0] {
		return URIEvent{}, abi.SigMismatch
	}
	if len(l.Topics[1:]) != URINumIndexed {
		return URIEvent{}, abi.IndexMismatch
	}
	item, _, err := abi.Decode(l.Data, URISchema)
	if err != nil {
		return URIEvent{}, err
	}
	res := DecodeURIEvent(item)
	res.Id = abi.Bytes(l.Topics[1][:]).BigInt()
	return res, nil
}

type BalanceOfRequest struct {
	item    *abi.Item
	Account [20]byte
	Id      *big.Int
}

func (x BalanceOfRequest) Done() {
	x.item.Done()
}

func DecodeBalanceOfRequest(item *abi.Item) BalanceOfRequest {
	x := BalanceOfRequest{}
	x.item = item
	x.Account = item.At(0).Address()
	x.Id = item.At(1).BigInt()
	return x
}

func (x BalanceOfRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.Address(x.Account)
	items[1] = abi.BigInt(x.Id)
	return abi.Tuple(items...)
}

type BalanceOfResponse struct {
	item      *abi.Item
	BalanceOf *big.Int
}

func (x BalanceOfResponse) Done() {
	x.item.Done()
}

func DecodeBalanceOfResponse(item *abi.Item) BalanceOfResponse {
	x := BalanceOfResponse{}
	x.item = item
	x.BalanceOf = item.At(0).BigInt()
	return x
}

func (x BalanceOfResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.BalanceOf)
	return abi.Tuple(items...)
}

var (
	BalanceOfRequestSignature = [32]byte{0x0, 0xfd, 0xd5, 0x8e, 0xa0, 0x32, 0x5f, 0xd7, 0x9f, 0x48, 0x6f, 0x80, 0x8, 0xad, 0x3f, 0xad, 0x17, 0xdc, 0xb1, 0xcd, 0x2e, 0xe8, 0x47, 0x42, 0x15, 0xc1, 0x14, 0x77, 0x1d, 0x87, 0x86, 0x3e}
	BalanceOfResponseSchema   = schema.Parse("(uint256)")
)

func CallBalanceOf(c *jrpc.Client, contract [20]byte, req BalanceOfRequest) (BalanceOfResponse, error) {
	var (
		s4 = BalanceOfRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return BalanceOfResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, BalanceOfResponseSchema)
	defer respItem.Done()
	if err != nil {
		return BalanceOfResponse{}, err
	}
	return DecodeBalanceOfResponse(respItem), nil
}

type BalanceOfBatchRequest struct {
	item     *abi.Item
	Accounts [][20]byte
	Ids      []*big.Int
}

func (x BalanceOfBatchRequest) Done() {
	x.item.Done()
}

func DecodeBalanceOfBatchRequest(item *abi.Item) BalanceOfBatchRequest {
	x := BalanceOfBatchRequest{}
	x.item = item
	var (
		accountsItem0 = item.At(0)
		accounts0     = make([][20]byte, accountsItem0.Len())
	)
	for i0 := 0; i0 < accountsItem0.Len(); i0++ {
		accounts0[i0] = accountsItem0.At(i0).Address()
	}
	x.Accounts = accounts0
	var (
		idsItem0 = item.At(1)
		ids0     = make([]*big.Int, idsItem0.Len())
	)
	for i0 := 0; i0 < idsItem0.Len(); i0++ {
		ids0[i0] = idsItem0.At(i0).BigInt()
	}
	x.Ids = ids0
	return x
}

func (x BalanceOfBatchRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	var (
		accounts0      = x.Accounts
		accountsItems0 = make([]*abi.Item, len(accounts0))
	)
	for i0 := 0; i0 < len(accounts0); i0++ {
		accountsItems0[i0] = abi.Address(accounts0[i0])
	}
	items[0] = abi.Array(accountsItems0...)
	var (
		ids0      = x.Ids
		idsItems0 = make([]*abi.Item, len(ids0))
	)
	for i0 := 0; i0 < len(ids0); i0++ {
		idsItems0[i0] = abi.BigInt(ids0[i0])
	}
	items[1] = abi.Array(idsItems0...)
	return abi.Tuple(items...)
}

type BalanceOfBatchResponse struct {
	item           *abi.Item
	BalanceOfBatch []*big.Int
}

func (x BalanceOfBatchResponse) Done() {
	x.item.Done()
}

func DecodeBalanceOfBatchResponse(item *abi.Item) BalanceOfBatchResponse {
	x := BalanceOfBatchResponse{}
	x.item = item
	var (
		balanceOfBatchItem0 = item.At(0)
		balanceOfBatch0     = make([]*big.Int, balanceOfBatchItem0.Len())
	)
	for i0 := 0; i0 < balanceOfBatchItem0.Len(); i0++ {
		balanceOfBatch0[i0] = balanceOfBatchItem0.At(i0).BigInt()
	}
	x.BalanceOfBatch = balanceOfBatch0
	return x
}

func (x BalanceOfBatchResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	var (
		balanceOfBatch0      = x.BalanceOfBatch
		balanceOfBatchItems0 = make([]*abi.Item, len(balanceOfBatch0))
	)
	for i0 := 0; i0 < len(balanceOfBatch0); i0++ {
		balanceOfBatchItems0[i0] = abi.BigInt(balanceOfBatch0[i0])
	}
	items[0] = abi.Array(balanceOfBatchItems0...)
	return abi.Tuple(items...)
}

var (
	BalanceOfBatchRequestSignature = [32]byte{0x4e, 0x12, 0x73, 0xf4, 0x6c, 0x22, 0x92, 0x33, 0xff, 0xb4, 0x64, 0xc9, 0x59, 0x61, 0x31, 0x91, 0x79, 0x15, 0x84, 0x81, 0x24, 0xc0, 0xe2, 0xe0, 0x1d, 0xdc, 0xbd, 0x31, 0xb, 0x26, 0x9, 0xd4}
	BalanceOfBatchResponseSchema   = schema.Parse("(uint256[])")
)

func CallBalanceOfBatch(c *jrpc.Client, contract [20]byte, req BalanceOfBatchRequest) (BalanceOfBatchResponse, error) {
	var (
		s4 = BalanceOfBatchRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return BalanceOfBatchResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, BalanceOfBatchResponseSchema)
	defer respItem.Done()
	if err != nil {
		return BalanceOfBatchResponse{}, err
	}
	return DecodeBalanceOfBatchResponse(respItem), nil
}

type IsApprovedForAllRequest struct {
	item     *abi.Item
	Account  [20]byte
	Operator [20]byte
}

func (x IsApprovedForAllRequest) Done() {
	x.item.Done()
}

func DecodeIsApprovedForAllRequest(item *abi.Item) IsApprovedForAllRequest {
	x := IsApprovedForAllRequest{}
	x.item = item
	x.Account = item.At(0).Address()
	x.Operator = item.At(1).Address()
	return x
}

func (x IsApprovedForAllRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.Address(x.Account)
	items[1] = abi.Address(x.Operator)
	return abi.Tuple(items...)
}

type IsApprovedForAllResponse struct {
	item             *abi.Item
	IsApprovedForAll bool
}

func (x IsApprovedForAllResponse) Done() {
	x.item.Done()
}

func DecodeIsApprovedForAllResponse(item *abi.Item) IsApprovedForAllResponse {
	x := IsApprovedForAllResponse{}
	x.item = item
	x.IsApprovedForAll = item.At(0).Bool()
	return x
}

func (x IsApprovedForAllResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.Bool(x.IsApprovedForAll)
	return abi.Tuple(items...)
}

var (
	IsApprovedForAllRequestSignature = [32]byte{0xe9, 0x85, 0xe9, 0xc5, 0xc6, 0x63, 0x6c, 0x68, 0x79, 0x25, 0x60, 0x1, 0x5, 0x7b, 0x28, 0xcc, 0xac, 0x77, 0x18, 0xef, 0xa, 0xc5, 0x65, 0x53, 0xff, 0x9b, 0x92, 0x64, 0x52, 0xca, 0xb8, 0xa3}
	IsApprovedForAllResponseSchema   = schema.Parse("(bool)")
)

func CallIsApprovedForAll(c *jrpc.Client, contract [20]byte, req IsApprovedForAllRequest) (IsApprovedForAllResponse, error) {
	var (
		s4 = IsApprovedForAllRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return IsApprovedForAllResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, IsApprovedForAllResponseSchema)
	defer respItem.Done()
	if err != nil {
		return IsApprovedForAllResponse{}, err
	}
	return DecodeIsApprovedForAllResponse(respItem), nil
}

type SafeBatchTransferFromRequest struct {
	item    *abi.Item
	From    [20]byte
	To      [20]byte
	Ids     []*big.Int
	Amounts []*big.Int
	Data    []byte
}

func (x SafeBatchTransferFromRequest) Done() {
	x.item.Done()
}

func DecodeSafeBatchTransferFromRequest(item *abi.Item) SafeBatchTransferFromRequest {
	x := SafeBatchTransferFromRequest{}
	x.item = item
	x.From = item.At(0).Address()
	x.To = item.At(1).Address()
	var (
		idsItem0 = item.At(2)
		ids0     = make([]*big.Int, idsItem0.Len())
	)
	for i0 := 0; i0 < idsItem0.Len(); i0++ {
		ids0[i0] = idsItem0.At(i0).BigInt()
	}
	x.Ids = ids0
	var (
		amountsItem0 = item.At(3)
		amounts0     = make([]*big.Int, amountsItem0.Len())
	)
	for i0 := 0; i0 < amountsItem0.Len(); i0++ {
		amounts0[i0] = amountsItem0.At(i0).BigInt()
	}
	x.Amounts = amounts0
	x.Data = item.At(4).Bytes()
	return x
}

func (x SafeBatchTransferFromRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 5)
	items[0] = abi.Address(x.From)
	items[1] = abi.Address(x.To)
	var (
		ids0      = x.Ids
		idsItems0 = make([]*abi.Item, len(ids0))
	)
	for i0 := 0; i0 < len(ids0); i0++ {
		idsItems0[i0] = abi.BigInt(ids0[i0])
	}
	items[2] = abi.Array(idsItems0...)
	var (
		amounts0      = x.Amounts
		amountsItems0 = make([]*abi.Item, len(amounts0))
	)
	for i0 := 0; i0 < len(amounts0); i0++ {
		amountsItems0[i0] = abi.BigInt(amounts0[i0])
	}
	items[3] = abi.Array(amountsItems0...)
	items[4] = abi.Bytes(x.Data)
	return abi.Tuple(items...)
}

var (
	SafeBatchTransferFromRequestSignature = [32]byte{0x2e, 0xb2, 0xc2, 0xd6, 0x67, 0xcf, 0x4, 0x8e, 0x1e, 0xc8, 0x2a, 0x45, 0x37, 0xb1, 0x23, 0xe4, 0x59, 0x95, 0x17, 0x29, 0xe0, 0x9e, 0xc7, 0x37, 0xac, 0x88, 0xa8, 0xe8, 0x2f, 0xb2, 0xd1, 0xdb}
	SafeBatchTransferFromResponseSchema   = schema.Parse("()")
)

type SafeTransferFromRequest struct {
	item   *abi.Item
	From   [20]byte
	To     [20]byte
	Id     *big.Int
	Amount *big.Int
	Data   []byte
}

func (x SafeTransferFromRequest) Done() {
	x.item.Done()
}

func DecodeSafeTransferFromRequest(item *abi.Item) SafeTransferFromRequest {
	x := SafeTransferFromRequest{}
	x.item = item
	x.From = item.At(0).Address()
	x.To = item.At(1).Address()
	x.Id = item.At(2).BigInt()
	x.Amount = item.At(3).BigInt()
	x.Data = item.At(4).Bytes()
	return x
}

func (x SafeTransferFromRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 5)
	items[0] = abi.Address(x.From)
	items[1] = abi.Address(x.To)
	items[2] = abi.BigInt(x.Id)
	items[3] = abi.BigInt(x.Amount)
	items[4] = abi.Bytes(x.Data)
	return abi.Tuple(items...)
}

var (
	SafeTransferFromRequestSignature = [32]byte{0xf2, 0x42, 0x43, 0x2a, 0x1, 0x95, 0x4b, 0xe, 0xe, 0xfb, 0x67, 0xe7, 0x2c, 0x9b, 0x3b, 0x8e, 0xd7, 0x76, 0x90, 0x65, 0x77, 0x80, 0x38, 0x5b, 0x25, 0x6a, 0xc9, 0xab, 0xa0, 0xe3, 0x5f, 0xb}
	SafeTransferFromResponseSchema   = schema.Parse("()")
)

type SetApprovalForAllRequest struct {
	item     *abi.Item
	Operator [20]byte
	Approved bool
}

func (x SetApprovalForAllRequest) Done() {
	x.item.Done()
}

func DecodeSetApprovalForAllRequest(item *abi.Item) SetApprovalForAllRequest {
	x := SetApprovalForAllRequest{}
	x.item = item
	x.Operator = item.At(0).Address()
	x.Approved = item.At(1).Bool()
	return x
}

func (x SetApprovalForAllRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.Address(x.Operator)
	items[1] = abi.Bool(x.Approved)
	return abi.Tuple(items...)
}

var (
	SetApprovalForAllRequestSignature = [32]byte{0xa2, 0x2c, 0xb4, 0x65, 0x1a, 0xb9, 0x57, 0xf, 0x89, 0xbb, 0x51, 0x63, 0x80, 0xc4, 0xc, 0xe7, 0x67, 0x62, 0x28, 0x4f, 0xb1, 0xf2, 0x13, 0x37, 0xce, 0xaf, 0x6a, 0xda, 0xb9, 0x9e, 0x7d, 0x4a}
	SetApprovalForAllResponseSchema   = schema.Parse("()")
)

type SupportsInterfaceRequest struct {
	item        *abi.Item
	InterfaceId [4]byte
}

func (x SupportsInterfaceRequest) Done() {
	x.item.Done()
}

func DecodeSupportsInterfaceRequest(item *abi.Item) SupportsInterfaceRequest {
	x := SupportsInterfaceRequest{}
	x.item = item
	x.InterfaceId = item.At(0).Bytes4()
	return x
}

func (x SupportsInterfaceRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.Bytes4(x.InterfaceId)
	return abi.Tuple(items...)
}

type SupportsInterfaceResponse struct {
	item              *abi.Item
	SupportsInterface bool
}

func (x SupportsInterfaceResponse) Done() {
	x.item.Done()
}

func DecodeSupportsInterfaceResponse(item *abi.Item) SupportsInterfaceResponse {
	x := SupportsInterfaceResponse{}
	x.item = item
	x.SupportsInterface = item.At(0).Bool()
	return x
}

func (x SupportsInterfaceResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.Bool(x.SupportsInterface)
	return abi.Tuple(items...)
}

var (
	SupportsInterfaceRequestSignature = [32]byte{0x1, 0xff, 0xc9, 0xa7, 0xa5, 0xce, 0xf8, 0xba, 0xa2, 0x1e, 0xd3, 0xc5, 0xc0, 0xd7, 0xe2, 0x3a, 0xcc, 0xb8, 0x4, 0xb6, 0x19, 0xe9, 0x33, 0x3b, 0x59, 0x7f, 0x47, 0xa0, 0xd8, 0x40, 0x76, 0xe2}
	SupportsInterfaceResponseSchema   = schema.Parse("(bool)")
)

func CallSupportsInterface(c *jrpc.Client, contract [20]byte, req SupportsInterfaceRequest) (SupportsInterfaceResponse, error) {
	var (
		s4 = SupportsInterfaceRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return SupportsInterfaceResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, SupportsInterfaceResponseSchema)
	defer respItem.Done()
	if err != nil {
		return SupportsInterfaceResponse{}, err
	}
	return DecodeSupportsInterfaceResponse(respItem), nil
}

type UriRequest struct {
	item *abi.Item
	Id   *big.Int
}

func (x UriRequest) Done() {
	x.item.Done()
}

func DecodeUriRequest(item *abi.Item) UriRequest {
	x := UriRequest{}
	x.item = item
	x.Id = item.At(0).BigInt()
	return x
}

func (x UriRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.Id)
	return abi.Tuple(items...)
}

type UriResponse struct {
	item *abi.Item
	Uri  string
}

func (x UriResponse) Done() {
	x.item.Done()
}

func DecodeUriResponse(item *abi.Item) UriResponse {
	x := UriResponse{}
	x.item = item
	x.Uri = item.At(0).String()
	return x
}

func (x UriResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.String(x.Uri)
	return abi.Tuple(items...)
}

var (
	UriRequestSignature = [32]byte{0xe, 0x89, 0x34, 0x1c, 0x5b, 0x74, 0x31, 0xe9, 0x52, 0x82, 0x62, 0x1b, 0xb9, 0xc5, 0x4e, 0x51, 0xfb, 0x5c, 0x29, 0x23, 0x4d, 0xf4, 0x3f, 0x9e, 0x19, 0x15, 0x1d, 0x38, 0x92, 0xfb, 0x3, 0x80}
	UriResponseSchema   = schema.Parse("(string)")
)

func CallUri(c *jrpc.Client, contract [20]byte, req UriRequest) (UriResponse, error) {
	var (
		s4 = UriRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return UriResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, UriResponseSchema)
	defer respItem.Done()
	if err != nil {
		return UriResponse{}, err
	}
	return DecodeUriResponse(respItem), nil
}

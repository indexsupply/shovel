// View functions and log parsing from erc721.json
//
// Code generated by "genabi"; DO NOT EDIT.
package erc721

import (
	"bytes"
	"github.com/indexsupply/x/abi"
	"github.com/indexsupply/x/abi/schema"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/jrpc"
	"math/big"
)

type ApprovalEvent struct {
	item     *abi.Item
	Owner    [20]byte
	Approved [20]byte
	TokenId  *big.Int
}

func (x ApprovalEvent) Done() {
	x.item.Done()
}

func (x ApprovalEvent) Encode() *abi.Item {
	items := make([]*abi.Item, 3)
	items[0] = abi.Address(x.Owner)
	items[1] = abi.Address(x.Approved)
	items[2] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

var (
	// Pre-compute for efficient bloom filter checks
	ApprovalSignatureHash = []byte{0xbe, 0x15, 0x80, 0xf4, 0x62, 0xf9, 0xf5, 0x97, 0x96, 0x31, 0x15, 0x11, 0xd1, 0x50, 0xe6, 0x84, 0x6c, 0x1d, 0x19, 0xc7, 0x62, 0xa1, 0xc9, 0x43, 0x4e, 0xf5, 0x42, 0xbc, 0x2b, 0x73, 0xda, 0xf6}
	ApprovalSignature     = []byte{0x8c, 0x5b, 0xe1, 0xe5, 0xeb, 0xec, 0x7d, 0x5b, 0xd1, 0x4f, 0x71, 0x42, 0x7d, 0x1e, 0x84, 0xf3, 0xdd, 0x3, 0x14, 0xc0, 0xf7, 0xb2, 0x29, 0x1e, 0x5b, 0x20, 0xa, 0xc8, 0xc7, 0xc3, 0xb9, 0x25}
	ApprovalSchema        = schema.Parse("()")
	ApprovalNumIndexed    = int(3)
)

// Event Signature:
//	Approval(address,address,uint256)
// Checks the first log topic against the signature hash:
//	8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925
//
// Copies indexed event inputs from the remaining topics
// into [Approval]
//
// Uses the the following abi schema to decode the un-indexed
// event inputs from the log's data field into [Approval]:
//	()
func MatchApproval(l *eth.Log) (ApprovalEvent, error) {
	if len(l.Topics) <= 0 {
		return ApprovalEvent{}, abi.NoTopics
	}
	if !bytes.Equal(ApprovalSignature, l.Topics[0]) {
		return ApprovalEvent{}, abi.SigMismatch
	}
	if len(l.Topics)-1 != ApprovalNumIndexed {
		return ApprovalEvent{}, abi.IndexMismatch
	}
	res := ApprovalEvent{}
	res.Owner = abi.Bytes(l.Topics[1]).Address()
	res.Approved = abi.Bytes(l.Topics[2]).Address()
	res.TokenId = abi.Bytes(l.Topics[3]).BigInt()
	return res, nil
}

type ApprovalForAllEvent struct {
	item     *abi.Item
	Owner    [20]byte
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
	items[0] = abi.Address(x.Owner)
	items[1] = abi.Address(x.Operator)
	items[2] = abi.Bool(x.Approved)
	return abi.Tuple(items...)
}

var (
	// Pre-compute for efficient bloom filter checks
	ApprovalForAllSignatureHash = []byte{0x8, 0xed, 0x60, 0xe3, 0xec, 0xf9, 0x50, 0xb2, 0xbd, 0xf1, 0xfe, 0xb3, 0x9d, 0xab, 0x31, 0xf3, 0x86, 0x38, 0x61, 0xf5, 0x7, 0x16, 0x62, 0x76, 0xde, 0x14, 0x8c, 0x22, 0xad, 0xc3, 0x84, 0x94}
	ApprovalForAllSignature     = []byte{0x17, 0x30, 0x7e, 0xab, 0x39, 0xab, 0x61, 0x7, 0xe8, 0x89, 0x98, 0x45, 0xad, 0x3d, 0x59, 0xbd, 0x96, 0x53, 0xf2, 0x0, 0xf2, 0x20, 0x92, 0x4, 0x89, 0xca, 0x2b, 0x59, 0x37, 0x69, 0x6c, 0x31}
	ApprovalForAllSchema        = schema.Parse("(bool)")
	ApprovalForAllNumIndexed    = int(2)
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
func MatchApprovalForAll(l *eth.Log) (ApprovalForAllEvent, error) {
	if len(l.Topics) <= 0 {
		return ApprovalForAllEvent{}, abi.NoTopics
	}
	if !bytes.Equal(ApprovalForAllSignature, l.Topics[0]) {
		return ApprovalForAllEvent{}, abi.SigMismatch
	}
	if len(l.Topics)-1 != ApprovalForAllNumIndexed {
		return ApprovalForAllEvent{}, abi.IndexMismatch
	}
	item, _, err := abi.Decode(l.Data, ApprovalForAllSchema)
	if err != nil {
		return ApprovalForAllEvent{}, err
	}
	res := DecodeApprovalForAllEvent(item)
	res.Owner = abi.Bytes(l.Topics[1]).Address()
	res.Operator = abi.Bytes(l.Topics[2]).Address()
	return res, nil
}

type TransferEvent struct {
	item    *abi.Item
	From    [20]byte
	To      [20]byte
	TokenId *big.Int
}

func (x TransferEvent) Done() {
	x.item.Done()
}

func (x TransferEvent) Encode() *abi.Item {
	items := make([]*abi.Item, 3)
	items[0] = abi.Address(x.From)
	items[1] = abi.Address(x.To)
	items[2] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

var (
	// Pre-compute for efficient bloom filter checks
	TransferSignatureHash = []byte{0xad, 0xa3, 0x89, 0xe1, 0xfc, 0x24, 0xa8, 0x58, 0x7c, 0x77, 0x63, 0x40, 0xef, 0xb9, 0x1b, 0x36, 0xe6, 0x75, 0x79, 0x2a, 0xb6, 0x31, 0x81, 0x61, 0x0, 0xd5, 0x5d, 0xf0, 0xb5, 0xcf, 0x3c, 0xbc}
	TransferSignature     = []byte{0xdd, 0xf2, 0x52, 0xad, 0x1b, 0xe2, 0xc8, 0x9b, 0x69, 0xc2, 0xb0, 0x68, 0xfc, 0x37, 0x8d, 0xaa, 0x95, 0x2b, 0xa7, 0xf1, 0x63, 0xc4, 0xa1, 0x16, 0x28, 0xf5, 0x5a, 0x4d, 0xf5, 0x23, 0xb3, 0xef}
	TransferSchema        = schema.Parse("()")
	TransferNumIndexed    = int(3)
)

// Event Signature:
//	Transfer(address,address,uint256)
// Checks the first log topic against the signature hash:
//	ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
//
// Copies indexed event inputs from the remaining topics
// into [Transfer]
//
// Uses the the following abi schema to decode the un-indexed
// event inputs from the log's data field into [Transfer]:
//	()
func MatchTransfer(l *eth.Log) (TransferEvent, error) {
	if len(l.Topics) <= 0 {
		return TransferEvent{}, abi.NoTopics
	}
	if !bytes.Equal(TransferSignature, l.Topics[0]) {
		return TransferEvent{}, abi.SigMismatch
	}
	if len(l.Topics)-1 != TransferNumIndexed {
		return TransferEvent{}, abi.IndexMismatch
	}
	res := TransferEvent{}
	res.From = abi.Bytes(l.Topics[1]).Address()
	res.To = abi.Bytes(l.Topics[2]).Address()
	res.TokenId = abi.Bytes(l.Topics[3]).BigInt()
	return res, nil
}

type ApproveRequest struct {
	item    *abi.Item
	To      [20]byte
	TokenId *big.Int
}

func (x ApproveRequest) Done() {
	x.item.Done()
}

func DecodeApproveRequest(item *abi.Item) ApproveRequest {
	x := ApproveRequest{}
	x.item = item
	x.To = item.At(0).Address()
	x.TokenId = item.At(1).BigInt()
	return x
}

func (x ApproveRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.Address(x.To)
	items[1] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

var (
	ApproveRequestSignature = []byte{0x9, 0x5e, 0xa7, 0xb3, 0x34, 0xae, 0x44, 0x0, 0x9a, 0xa8, 0x67, 0xbf, 0xb3, 0x86, 0xf5, 0xc3, 0xb4, 0xb4, 0x43, 0xac, 0x6f, 0xe, 0xe5, 0x73, 0xfa, 0x91, 0xc4, 0x60, 0x8f, 0xba, 0xdf, 0xba}
	ApproveResponseSchema   = schema.Parse("()")
)

type TotalSupplyResponse struct {
	item        *abi.Item
	TotalSupply *big.Int
}

func (x TotalSupplyResponse) Done() {
	x.item.Done()
}

func DecodeTotalSupplyResponse(item *abi.Item) TotalSupplyResponse {
	x := TotalSupplyResponse{}
	x.item = item
	x.TotalSupply = item.At(0).BigInt()
	return x
}

func (x TotalSupplyResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.TotalSupply)
	return abi.Tuple(items...)
}

var (
	TotalSupplyRequestSignature = []byte{0x18, 0x16, 0xd, 0xdd, 0x7f, 0x15, 0xc7, 0x25, 0x28, 0xc2, 0xf9, 0x4f, 0xd8, 0xdf, 0xe3, 0xc8, 0xd5, 0xaa, 0x26, 0xe2, 0xc5, 0xc, 0x7d, 0x81, 0xf4, 0xbc, 0x7b, 0xee, 0x8d, 0x4b, 0x79, 0x32}
	TotalSupplyResponseSchema   = schema.Parse("(uint256)")
)

func CallTotalSupply(c *jrpc.Client, contract [20]byte) (TotalSupplyResponse, error) {
	respData, err := c.EthCall(contract, TotalSupplyRequestSignature[:4])
	if err != nil {
		return TotalSupplyResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, TotalSupplyResponseSchema)
	defer respItem.Done()
	if err != nil {
		return TotalSupplyResponse{}, err
	}
	return DecodeTotalSupplyResponse(respItem), nil
}

type BalanceOfRequest struct {
	item  *abi.Item
	Owner [20]byte
}

func (x BalanceOfRequest) Done() {
	x.item.Done()
}

func DecodeBalanceOfRequest(item *abi.Item) BalanceOfRequest {
	x := BalanceOfRequest{}
	x.item = item
	x.Owner = item.At(0).Address()
	return x
}

func (x BalanceOfRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.Address(x.Owner)
	return abi.Tuple(items...)
}

type BalanceOfResponse struct {
	item    *abi.Item
	Balance *big.Int
}

func (x BalanceOfResponse) Done() {
	x.item.Done()
}

func DecodeBalanceOfResponse(item *abi.Item) BalanceOfResponse {
	x := BalanceOfResponse{}
	x.item = item
	x.Balance = item.At(0).BigInt()
	return x
}

func (x BalanceOfResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.Balance)
	return abi.Tuple(items...)
}

var (
	BalanceOfRequestSignature = []byte{0x70, 0xa0, 0x82, 0x31, 0xb9, 0x8e, 0xf4, 0xca, 0x26, 0x8c, 0x9c, 0xc3, 0xf6, 0xb4, 0x59, 0xe, 0x4b, 0xfe, 0xc2, 0x82, 0x80, 0xdb, 0x6, 0xbb, 0x5d, 0x45, 0xe6, 0x89, 0xf2, 0xa3, 0x60, 0xbe}
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

type GetApprovedRequest struct {
	item    *abi.Item
	TokenId *big.Int
}

func (x GetApprovedRequest) Done() {
	x.item.Done()
}

func DecodeGetApprovedRequest(item *abi.Item) GetApprovedRequest {
	x := GetApprovedRequest{}
	x.item = item
	x.TokenId = item.At(0).BigInt()
	return x
}

func (x GetApprovedRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

type GetApprovedResponse struct {
	item     *abi.Item
	Operator [20]byte
}

func (x GetApprovedResponse) Done() {
	x.item.Done()
}

func DecodeGetApprovedResponse(item *abi.Item) GetApprovedResponse {
	x := GetApprovedResponse{}
	x.item = item
	x.Operator = item.At(0).Address()
	return x
}

func (x GetApprovedResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.Address(x.Operator)
	return abi.Tuple(items...)
}

var (
	GetApprovedRequestSignature = []byte{0x8, 0x18, 0x12, 0xfc, 0x55, 0xe3, 0x4f, 0xdc, 0x7c, 0xf5, 0xd8, 0xb5, 0xcf, 0x4e, 0x36, 0x21, 0xfa, 0x64, 0x23, 0xfd, 0xe9, 0x52, 0xec, 0x6a, 0xb2, 0x4a, 0xfd, 0xc0, 0xd8, 0x5c, 0xb, 0x2e}
	GetApprovedResponseSchema   = schema.Parse("(address)")
)

func CallGetApproved(c *jrpc.Client, contract [20]byte, req GetApprovedRequest) (GetApprovedResponse, error) {
	var (
		s4 = GetApprovedRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return GetApprovedResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, GetApprovedResponseSchema)
	defer respItem.Done()
	if err != nil {
		return GetApprovedResponse{}, err
	}
	return DecodeGetApprovedResponse(respItem), nil
}

type IsApprovedForAllRequest struct {
	item     *abi.Item
	Owner    [20]byte
	Operator [20]byte
}

func (x IsApprovedForAllRequest) Done() {
	x.item.Done()
}

func DecodeIsApprovedForAllRequest(item *abi.Item) IsApprovedForAllRequest {
	x := IsApprovedForAllRequest{}
	x.item = item
	x.Owner = item.At(0).Address()
	x.Operator = item.At(1).Address()
	return x
}

func (x IsApprovedForAllRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.Address(x.Owner)
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
	IsApprovedForAllRequestSignature = []byte{0xe9, 0x85, 0xe9, 0xc5, 0xc6, 0x63, 0x6c, 0x68, 0x79, 0x25, 0x60, 0x1, 0x5, 0x7b, 0x28, 0xcc, 0xac, 0x77, 0x18, 0xef, 0xa, 0xc5, 0x65, 0x53, 0xff, 0x9b, 0x92, 0x64, 0x52, 0xca, 0xb8, 0xa3}
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

type NameResponse struct {
	item *abi.Item
	Name string
}

func (x NameResponse) Done() {
	x.item.Done()
}

func DecodeNameResponse(item *abi.Item) NameResponse {
	x := NameResponse{}
	x.item = item
	x.Name = item.At(0).String()
	return x
}

func (x NameResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.String(x.Name)
	return abi.Tuple(items...)
}

var (
	NameRequestSignature = []byte{0x6, 0xfd, 0xde, 0x3, 0x83, 0xf1, 0x5d, 0x58, 0x2d, 0x1a, 0x74, 0x51, 0x14, 0x86, 0xc9, 0xdd, 0xf8, 0x62, 0xa8, 0x82, 0xfb, 0x79, 0x4, 0xb3, 0xd9, 0xfe, 0x9b, 0x8b, 0x8e, 0x58, 0xa7, 0x96}
	NameResponseSchema   = schema.Parse("(string)")
)

func CallName(c *jrpc.Client, contract [20]byte) (NameResponse, error) {
	respData, err := c.EthCall(contract, NameRequestSignature[:4])
	if err != nil {
		return NameResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, NameResponseSchema)
	defer respItem.Done()
	if err != nil {
		return NameResponse{}, err
	}
	return DecodeNameResponse(respItem), nil
}

type OwnerOfRequest struct {
	item    *abi.Item
	TokenId *big.Int
}

func (x OwnerOfRequest) Done() {
	x.item.Done()
}

func DecodeOwnerOfRequest(item *abi.Item) OwnerOfRequest {
	x := OwnerOfRequest{}
	x.item = item
	x.TokenId = item.At(0).BigInt()
	return x
}

func (x OwnerOfRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

type OwnerOfResponse struct {
	item  *abi.Item
	Owner [20]byte
}

func (x OwnerOfResponse) Done() {
	x.item.Done()
}

func DecodeOwnerOfResponse(item *abi.Item) OwnerOfResponse {
	x := OwnerOfResponse{}
	x.item = item
	x.Owner = item.At(0).Address()
	return x
}

func (x OwnerOfResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.Address(x.Owner)
	return abi.Tuple(items...)
}

var (
	OwnerOfRequestSignature = []byte{0x63, 0x52, 0x21, 0x1e, 0x65, 0x66, 0xaa, 0x2, 0x7e, 0x75, 0xac, 0x9d, 0xbf, 0x24, 0x23, 0x19, 0x7f, 0xbd, 0x9b, 0x82, 0xb9, 0xd9, 0x81, 0xa3, 0xab, 0x36, 0x7d, 0x35, 0x58, 0x66, 0xaa, 0x1c}
	OwnerOfResponseSchema   = schema.Parse("(address)")
)

func CallOwnerOf(c *jrpc.Client, contract [20]byte, req OwnerOfRequest) (OwnerOfResponse, error) {
	var (
		s4 = OwnerOfRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return OwnerOfResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, OwnerOfResponseSchema)
	defer respItem.Done()
	if err != nil {
		return OwnerOfResponse{}, err
	}
	return DecodeOwnerOfResponse(respItem), nil
}

type SafeTransferFromRequest struct {
	item    *abi.Item
	From    [20]byte
	To      [20]byte
	TokenId *big.Int
}

func (x SafeTransferFromRequest) Done() {
	x.item.Done()
}

func DecodeSafeTransferFromRequest(item *abi.Item) SafeTransferFromRequest {
	x := SafeTransferFromRequest{}
	x.item = item
	x.From = item.At(0).Address()
	x.To = item.At(1).Address()
	x.TokenId = item.At(2).BigInt()
	return x
}

func (x SafeTransferFromRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 3)
	items[0] = abi.Address(x.From)
	items[1] = abi.Address(x.To)
	items[2] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

var (
	SafeTransferFromRequestSignature = []byte{0x42, 0x84, 0x2e, 0xe, 0xb3, 0x88, 0x57, 0xa7, 0x77, 0x5b, 0x4e, 0x73, 0x64, 0xb2, 0x77, 0x5d, 0xf7, 0x32, 0x50, 0x74, 0xd0, 0x88, 0xe7, 0xfb, 0x39, 0x59, 0xc, 0xd6, 0x28, 0x11, 0x84, 0xed}
	SafeTransferFromResponseSchema   = schema.Parse("()")
)

type SafeTransferFrom2Request struct {
	item    *abi.Item
	From    [20]byte
	To      [20]byte
	TokenId *big.Int
	Data    []byte
}

func (x SafeTransferFrom2Request) Done() {
	x.item.Done()
}

func DecodeSafeTransferFrom2Request(item *abi.Item) SafeTransferFrom2Request {
	x := SafeTransferFrom2Request{}
	x.item = item
	x.From = item.At(0).Address()
	x.To = item.At(1).Address()
	x.TokenId = item.At(2).BigInt()
	x.Data = item.At(3).Bytes()
	return x
}

func (x SafeTransferFrom2Request) Encode() *abi.Item {
	items := make([]*abi.Item, 4)
	items[0] = abi.Address(x.From)
	items[1] = abi.Address(x.To)
	items[2] = abi.BigInt(x.TokenId)
	items[3] = abi.Bytes(x.Data)
	return abi.Tuple(items...)
}

var (
	SafeTransferFrom2RequestSignature = []byte{0xd0, 0xce, 0x5c, 0xa8, 0x6c, 0x59, 0xa9, 0xea, 0xf1, 0x59, 0x5, 0x25, 0xd8, 0xbf, 0x69, 0x6e, 0x7f, 0xb7, 0xa1, 0x29, 0x71, 0xbe, 0x34, 0xe7, 0xe4, 0x2f, 0xaa, 0xbc, 0xd3, 0xef, 0xed, 0x9d}
	SafeTransferFrom2ResponseSchema   = schema.Parse("()")
)

type SetApprovalForAllRequest struct {
	item      *abi.Item
	Operator  [20]byte
	_Approved bool
}

func (x SetApprovalForAllRequest) Done() {
	x.item.Done()
}

func DecodeSetApprovalForAllRequest(item *abi.Item) SetApprovalForAllRequest {
	x := SetApprovalForAllRequest{}
	x.item = item
	x.Operator = item.At(0).Address()
	x._Approved = item.At(1).Bool()
	return x
}

func (x SetApprovalForAllRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 2)
	items[0] = abi.Address(x.Operator)
	items[1] = abi.Bool(x._Approved)
	return abi.Tuple(items...)
}

var (
	SetApprovalForAllRequestSignature = []byte{0xa2, 0x2c, 0xb4, 0x65, 0x1a, 0xb9, 0x57, 0xf, 0x89, 0xbb, 0x51, 0x63, 0x80, 0xc4, 0xc, 0xe7, 0x67, 0x62, 0x28, 0x4f, 0xb1, 0xf2, 0x13, 0x37, 0xce, 0xaf, 0x6a, 0xda, 0xb9, 0x9e, 0x7d, 0x4a}
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
	SupportsInterfaceRequestSignature = []byte{0x1, 0xff, 0xc9, 0xa7, 0xa5, 0xce, 0xf8, 0xba, 0xa2, 0x1e, 0xd3, 0xc5, 0xc0, 0xd7, 0xe2, 0x3a, 0xcc, 0xb8, 0x4, 0xb6, 0x19, 0xe9, 0x33, 0x3b, 0x59, 0x7f, 0x47, 0xa0, 0xd8, 0x40, 0x76, 0xe2}
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

type SymbolResponse struct {
	item   *abi.Item
	Symbol string
}

func (x SymbolResponse) Done() {
	x.item.Done()
}

func DecodeSymbolResponse(item *abi.Item) SymbolResponse {
	x := SymbolResponse{}
	x.item = item
	x.Symbol = item.At(0).String()
	return x
}

func (x SymbolResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.String(x.Symbol)
	return abi.Tuple(items...)
}

var (
	SymbolRequestSignature = []byte{0x95, 0xd8, 0x9b, 0x41, 0xe2, 0xf5, 0xf3, 0x91, 0xa7, 0x9e, 0xc5, 0x4e, 0x9d, 0x87, 0xc7, 0x9d, 0x6e, 0x77, 0x7c, 0x63, 0xe3, 0x2c, 0x28, 0xda, 0x95, 0xb4, 0xe9, 0xe4, 0xa7, 0x92, 0x50, 0xec}
	SymbolResponseSchema   = schema.Parse("(string)")
)

func CallSymbol(c *jrpc.Client, contract [20]byte) (SymbolResponse, error) {
	respData, err := c.EthCall(contract, SymbolRequestSignature[:4])
	if err != nil {
		return SymbolResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, SymbolResponseSchema)
	defer respItem.Done()
	if err != nil {
		return SymbolResponse{}, err
	}
	return DecodeSymbolResponse(respItem), nil
}

type TokenURIRequest struct {
	item    *abi.Item
	TokenId *big.Int
}

func (x TokenURIRequest) Done() {
	x.item.Done()
}

func DecodeTokenURIRequest(item *abi.Item) TokenURIRequest {
	x := TokenURIRequest{}
	x.item = item
	x.TokenId = item.At(0).BigInt()
	return x
}

func (x TokenURIRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

type TokenURIResponse struct {
	item     *abi.Item
	TokenURI string
}

func (x TokenURIResponse) Done() {
	x.item.Done()
}

func DecodeTokenURIResponse(item *abi.Item) TokenURIResponse {
	x := TokenURIResponse{}
	x.item = item
	x.TokenURI = item.At(0).String()
	return x
}

func (x TokenURIResponse) Encode() *abi.Item {
	items := make([]*abi.Item, 1)
	items[0] = abi.String(x.TokenURI)
	return abi.Tuple(items...)
}

var (
	TokenURIRequestSignature = []byte{0xc8, 0x7b, 0x56, 0xdd, 0xa7, 0x52, 0x23, 0x2, 0x62, 0x93, 0x59, 0x40, 0xd9, 0x7, 0xf0, 0x47, 0xa9, 0xf8, 0x6b, 0xb5, 0xee, 0x6a, 0xa3, 0x35, 0x11, 0xfc, 0x86, 0xdb, 0x33, 0xfe, 0xa6, 0xcc}
	TokenURIResponseSchema   = schema.Parse("(string)")
)

func CallTokenURI(c *jrpc.Client, contract [20]byte, req TokenURIRequest) (TokenURIResponse, error) {
	var (
		s4 = TokenURIRequestSignature[:4]
		cd = append(s4, abi.Encode(req.Encode())...)
	)
	respData, err := c.EthCall(contract, cd)
	if err != nil {
		return TokenURIResponse{}, err
	}
	respItem, _, err := abi.Decode(respData, TokenURIResponseSchema)
	defer respItem.Done()
	if err != nil {
		return TokenURIResponse{}, err
	}
	return DecodeTokenURIResponse(respItem), nil
}

type TransferFromRequest struct {
	item    *abi.Item
	From    [20]byte
	To      [20]byte
	TokenId *big.Int
}

func (x TransferFromRequest) Done() {
	x.item.Done()
}

func DecodeTransferFromRequest(item *abi.Item) TransferFromRequest {
	x := TransferFromRequest{}
	x.item = item
	x.From = item.At(0).Address()
	x.To = item.At(1).Address()
	x.TokenId = item.At(2).BigInt()
	return x
}

func (x TransferFromRequest) Encode() *abi.Item {
	items := make([]*abi.Item, 3)
	items[0] = abi.Address(x.From)
	items[1] = abi.Address(x.To)
	items[2] = abi.BigInt(x.TokenId)
	return abi.Tuple(items...)
}

var (
	TransferFromRequestSignature = []byte{0x23, 0xb8, 0x72, 0xdd, 0x73, 0x2, 0x11, 0x33, 0x69, 0xcd, 0xa2, 0x90, 0x12, 0x43, 0x42, 0x94, 0x19, 0xbe, 0xc1, 0x45, 0x40, 0x8f, 0xa8, 0xb3, 0x52, 0xb3, 0xdd, 0x92, 0xb6, 0x6c, 0x68, 0xb}
	TransferFromResponseSchema   = schema.Parse("()")
)

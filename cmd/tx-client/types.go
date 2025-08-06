package main

// BlockTransactions 表示一个区块中的所有交易
type BlockTransactions struct {
	BlockNumber  uint64            `json:"block_number"`
	BlockHash    string            `json:"block_hash"`
	Transactions []TransactionData `json:"transactions"`
	Timestamp    uint64            `json:"timestamp"`
}

// TransactionData 存储交易的原始数据
type TransactionData struct {
	Hash string `json:"hash"`
	Raw  string `json:"raw"` // 十六进制编码的原始交易数据
} 
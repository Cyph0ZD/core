/*
File Name:  Block Encoding.go
Copyright:  2021 Peernet s.r.o.
Author:     Peter Kleissner

Block encoding in messages and local storage.
*/

package core

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/btcsuite/btcd/btcec"
)

// Block is a single block containing a set of records (metadata).
// It has no upper size limit, although a soft limit of 64 KB - overhead is encouraged for efficiency.
type Block struct {
	OwnerPublicKey    *btcec.PublicKey // Owner Public Key, ECDSA (secp256k1) 257-bit
	LastBlockHash     []byte           // Hash of the last block. Blake3.
	BlockchainVersion uint64           // Blockchain version
	Number            uint64           // Block number
	RecordsRaw        []BlockRecordRaw // Block records raw
}

// BlockRecordRaw is a single block record (not decoded)
type BlockRecordRaw struct {
	Type uint8  // Record Type. See RecordTypeX.
	Data []byte // Data according to the type
}

const blockHeaderSize = 115
const blockRecordHeaderSize = 5

// decodeBlock decodes a single block
func decodeBlock(raw []byte) (block *Block, err error) {
	if len(raw) < blockHeaderSize {
		return nil, errors.New("decodeBlock invalid block size")
	}

	block = &Block{}

	signature := raw[0 : 0+65]

	block.OwnerPublicKey, _, err = btcec.RecoverCompact(btcec.S256(), signature, hashData(raw[65:]))
	if err != nil {
		return nil, err
	}

	block.LastBlockHash = make([]byte, hashSize)
	copy(block.LastBlockHash, raw[65:65+hashSize])

	block.BlockchainVersion = binary.LittleEndian.Uint64(raw[97 : 97+8])
	block.Number = uint64(binary.LittleEndian.Uint32(raw[105 : 105+4])) // for now 32-bit in protocol

	blockSize := binary.LittleEndian.Uint32(raw[109 : 109+4])
	if blockSize != uint32(len(raw)) {
		return nil, errors.New("decodeBlock invalid block size")
	}

	// decode on a low-level all block records
	countRecords := binary.LittleEndian.Uint16(raw[113 : 113+2])
	index := 115

	for n := uint16(0); n < countRecords; n++ {
		if index+blockRecordHeaderSize > len(raw) {
			return nil, errors.New("decodeBlock block record exceeds block size")
		}

		recordType := raw[index]
		recordSize := binary.LittleEndian.Uint32(raw[index+1 : index+5])
		index += blockRecordHeaderSize

		if index+int(recordSize) > len(raw) {
			return nil, errors.New("decodeBlock block record exceeds block size")
		}

		block.RecordsRaw = append(block.RecordsRaw, BlockRecordRaw{Type: recordType, Data: raw[index : index+int(recordSize)]})

		index += int(recordSize)
	}

	return block, nil
}

func encodeBlock(block *Block, ownerPrivateKey *btcec.PrivateKey) (raw []byte, err error) {
	var buffer bytes.Buffer
	buffer.Write(make([]byte, 65)) // Signature, filled at the end

	if block.Number > 0 && len(block.LastBlockHash) != hashSize {
		return nil, errors.New("encodeBlock invalid last block hash")
	} else if block.Number == 0 { // Block 0: Empty last hash
		block.LastBlockHash = make([]byte, 32)
	}
	buffer.Write(block.LastBlockHash)

	var temp [8]byte
	binary.LittleEndian.PutUint64(temp[0:8], block.BlockchainVersion)
	buffer.Write(temp[:])

	binary.LittleEndian.PutUint32(temp[0:4], uint32(block.Number)) // for now 32-bit in protocol
	buffer.Write(temp[:4])

	buffer.Write(make([]byte, 4)) // Size of block, filled later
	buffer.Write(make([]byte, 2)) // Count of records, filled later

	// write all records
	countRecords := uint16(0)

	for _, record := range block.RecordsRaw {
		var temp [8]byte
		binary.LittleEndian.PutUint32(temp[0:4], uint32(len(record.Data)))

		buffer.Write([]byte{record.Type}) // Record Type
		buffer.Write(temp[:4])            // Size of data
		buffer.Write(record.Data)         // Data

		countRecords++
	}

	// finalize the block
	raw = buffer.Bytes()
	if len(raw) < blockHeaderSize {
		return nil, errors.New("encodeBlock invalid block size")
	}

	binary.LittleEndian.PutUint32(raw[109:109+4], uint32(len(raw))) // Size of block
	binary.LittleEndian.PutUint16(raw[113:113+2], countRecords)     // Count of records

	// signature is last
	signature, err := btcec.SignCompact(btcec.S256(), ownerPrivateKey, hashData(raw[65:]), true)
	if err != nil {
		return nil, err
	}
	copy(raw[0:0+65], signature)

	return raw, nil
}

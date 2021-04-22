/*
File Name:  Kademlia.go
Copyright:  2021 Peernet s.r.o.
Author:     Peter Kleissner
*/

package core

import (
	"bytes"
	"time"

	"github.com/PeernetOfficial/core/dht"
)

var nodesDHT *dht.DHT

func initKademlia() {
	nodesDHT = dht.NewDHT(&dht.Node{ID: nodeID}, 256, 20, 5)

	// ShouldEvict determines whether node 1 shall be evicted in favor of node 2
	nodesDHT.ShouldEvict = func(node1, node2 *dht.Node) bool {
		rttOld := node1.Info.(*PeerInfo).GetRTT()
		rttNew := node2.Info.(*PeerInfo).GetRTT()

		// evict the old node if the new one has a faster ping time
		if rttOld == 0 { // old one has no recent RTT (happens if all connections are inactive)?
			return true
		} else if rttNew > 0 {
			// If new RTT is smaller, evict old one.
			return rttNew < rttOld
		}

		// If here, none has a RTT. Keep the closer (by distance) one.
		return nodesDHT.IsNodeCloser(node1.ID, node2.ID)
	}

	// SendRequestStore sends a store message to the remote node. I.e. asking it to store the given key-value
	nodesDHT.SendRequestStore = func(node *dht.Node, key []byte, dataSize uint64) {
		node.Info.(*PeerInfo).sendAnnouncementStore(key, dataSize)
	}

	// SendRequestFindNode sends an information request to find a particular node. nodes are the nodes to send the request to.
	nodesDHT.SendRequestFindNode = func(request *dht.InformationRequest) {
		for _, node := range request.Nodes {
			node.Info.(*PeerInfo).sendAnnouncementFindNode(request)
		}
	}

	// SendRequestFindValue sends an information request to find data. nodes are the nodes to send the request to.
	nodesDHT.SendRequestFindValue = func(request *dht.InformationRequest) {
		for _, node := range request.Nodes {
			node.Info.(*PeerInfo).sendAnnouncementFindValue(request)
		}
	}
}

// Future sendAnnouncementX: If it detects that announcements are sent out to the same peer within 50ms it should activate a wait-and-group scheme.

func (peer *PeerInfo) sendAnnouncementFindNode(request *dht.InformationRequest) {
	// If the key is self, send it as FIND_SELF
	if bytes.Equal(request.Key, nodeID) {
		peer.sendAnnouncement(false, true, nil, nil, nil, request)
	} else {
		peer.sendAnnouncement(false, false, []KeyHash{{Hash: request.Key}}, nil, nil, request)
	}
}

func (peer *PeerInfo) sendAnnouncementFindValue(request *dht.InformationRequest) {

	findSelf := false
	var findPeer []KeyHash
	var findValue []KeyHash

	findValue = append(findValue, KeyHash{Hash: request.Key})

	peer.sendAnnouncement(false, findSelf, findPeer, findValue, nil, request)
}

func (peer *PeerInfo) sendAnnouncementStore(fileHash []byte, fileSize uint64) {
	peer.sendAnnouncement(false, false, nil, nil, []InfoStore{{ID: KeyHash{Hash: fileHash}, Size: fileSize, Type: 0}}, nil)
}

// ---- CORE DATA FUNCTIONS ----

// Data2Hash returns the hash for the data
func Data2Hash(data []byte) (hash []byte) {
	return hashData(data)
}

// GetData returns the requested data. It checks first the local store and then tries via DHT.
func GetData(hash []byte) (data []byte, senderNodeID []byte, found bool) {
	if data, found = GetDataLocal(hash); found {
		return data, nodeID, found
	}

	return GetDataDHT(hash)
}

// GetDataLocal returns data from the local warehouse.
func GetDataLocal(hash []byte) (data []byte, found bool) {
	return Warehouse.Retrieve(hash)
}

// GetDataDHT requests data via DHT
func GetDataDHT(hash []byte) (data []byte, senderNodeID []byte, found bool) {
	data, senderNodeID, found, _ = nodesDHT.Get(hash)
	return data, senderNodeID, found
}

// StoreDataLocal stores data into the local warehouse.
func StoreDataLocal(data []byte) error {
	key := hashData(data)
	return Warehouse.Store(key, data, time.Time{}, time.Time{})
}

// StoreDataDHT stores data locally and informs peers in the DHT about it.
// Remote peers may choose to keep a record (in case another peers asks) or mirror the full data.
func StoreDataDHT(data []byte) error {
	key := hashData(data)
	if err := Warehouse.Store(key, data, time.Time{}, time.Time{}); err != nil {
		return err
	}
	return nodesDHT.Store(key, uint64(len(data)))
}

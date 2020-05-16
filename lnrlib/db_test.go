package lndrlib

import (
	"fmt"
	"testing"
)

func TestSuggestAddress(t *testing.T) {
	var tests = []struct {
		a, b [4]byte
		seed [4]byte
		want [4]byte
	}{
		{[4]byte{0, 0, 0, 1}, [4]byte{0, 0, 0, 2}, [4]byte{0, 0, 0, 1}, [4]byte{0, 0, 0, 0}},
		{[4]byte{0, 0, 0, 1}, [4]byte{0, 0, 0, 2}, [4]byte{0, 0, 0, 2}, [4]byte{0, 0, 0, 3}},
		{[4]byte{0, 0, 0, 2}, [4]byte{0, 0, 0, 3}, [4]byte{0, 0, 0, 2}, [4]byte{0, 0, 0, 1}},
	}

	for _, test := range tests {
		testname := fmt.Sprintf("DB:%v,%v Seed:%v", test.a, test.b, test.seed)
		t.Run(testname, func(t *testing.T) {
			//Create database and load two addresses into it
			db := createDB("")
			addressInfoA := addressInfo{address: test.a, nodePubKey: [33]byte{},
				registrationHeight: 0, registrationTxID: [32]byte{}, version: 0}
			addressInfoB := addressInfo{address: test.b, nodePubKey: [33]byte{},
				registrationHeight: 0, registrationTxID: [32]byte{}, version: 0}
			db.addAddressToDB(&addressInfoA)
			db.addAddressToDB(&addressInfoB)

			suggestedAddress, err := db.SuggestAddress(test.seed)
			if err != nil {
				t.Errorf("TestSuggestAddress failed on SuggestAddress:" + err.Error())
			}

			if suggestedAddress != test.want {
				t.Errorf("TestSuggestAddress wants %v and got %v", test.want, suggestedAddress)
			}
		})
	}
}

func TestAddAddress(t *testing.T) {

	var tests = [][4]byte{
		{0, 0, 0, 0},
		{0, 0, 0, 5},
	}

	for _, testAddress := range tests {
		testname := fmt.Sprintf("%v", testAddress)
		t.Run(testname, func(t *testing.T) {
			//Create database and load two addresses into it
			db := createDB("")
			addressInfo := addressInfo{address: testAddress, nodePubKey: [33]byte{},
				registrationHeight: 0, registrationTxID: [32]byte{}, version: 0}
			db.addAddressToDB(&addressInfo)

			if !db.IsAddressRegistered(testAddress) {
				t.Errorf("TestLoadAddress failed to load address %v", testAddress)
			}
		})
	}
}

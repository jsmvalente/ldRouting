package ldrlib

import "container/list"

type routingStack list.List

func createRoutingStack() *routingStack {
	return (*routingStack)(list.New())
}

func (s *routingStack) empty() bool {
	return (*list.List)(s).Len() == 0
}

func (s *routingStack) peek() *routingEntry {
	return (*list.List)(s).Front().Value.(*routingEntry)
}

func (s *routingStack) peekFromBlock(fromBlock uint64) []*routingEntry {

	var retEntries []*routingEntry
	var entry *routingEntry

	//Find which entries of the stack we should send
	//Everything that was updated after 'fromBlock' block height should be sent
	for e := (*list.List)(s).Front(); e != nil; e = e.Next() {

		entry = (e.Value).(*routingEntry)

		if entry.height < fromBlock {
			break
		}

		retEntries = append(retEntries, entry)
	}

	return retEntries
}

func (s *routingStack) put(entry *routingEntry) {
	(*list.List)(s).PushFront(entry)
}

func (s *routingStack) pop() *routingEntry {

	front := (*list.List)(s).Front()
	return ((*list.List)(s).Remove(front)).(*routingEntry)
}

func (s *routingStack) remove(entry *routingEntry) {

	var e *list.Element
	//FInd the element to remove
	for e = (*list.List)(s).Front(); e != nil; e = e.Next() {
		if e.Value.(*routingEntry) == entry {
			break
		}
	}

	(*list.List)(s).Remove(e)
}

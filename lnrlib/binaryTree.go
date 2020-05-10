package lndrlib

type node struct {
	data  interface{}
	left  *node
	right *node
}

func createBinaryTree() *node {
	return &(node{})
}

//REturn a pointer to the left child of the node, nil if there is none
func (n *node) leftChild() *node {
	return n.left
}

//Create the left child
func (n *node) createLeftChild() *node {
	n.left = &(node{})

	return n.left
}

//REturn a pointer to the left child of the node, nil if there is none
func (n *node) rightChild() *node {
	return n.right
}

//Create the left child
func (n *node) createRightChild() *node {
	n.right = &(node{})

	return n.right
}

//REturn the data of the node
func (n *node) getData() interface{} {
	return n.data
}

func (n *node) saveData(data interface{}) {
	n.data = data
}

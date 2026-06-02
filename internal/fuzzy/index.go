package fuzzy

type bkNode struct {
	value    uint64
	indices  []int
	children map[int]*bkNode
}

type bkTree struct {
	root *bkNode
}

func (t *bkTree) Insert(value uint64, index int) {
	if t.root == nil {
		t.root = &bkNode{value: value, indices: []int{index}, children: make(map[int]*bkNode)}
		return
	}

	node := t.root
	for {
		distance := HammingDistance(value, node.value)
		if distance == 0 {
			node.indices = append(node.indices, index)
			return
		}
		child, ok := node.children[distance]
		if !ok {
			node.children[distance] = &bkNode{value: value, indices: []int{index}, children: make(map[int]*bkNode)}
			return
		}
		node = child
	}
}

func (t *bkTree) Search(target uint64, maxDistance int) []int {
	if t.root == nil {
		return nil
	}
	out := make([]int, 0, 16)
	searchNode(t.root, target, maxDistance, &out)
	return out
}

func searchNode(node *bkNode, target uint64, maxDistance int, out *[]int) {
	if node == nil {
		return
	}

	distance := HammingDistance(target, node.value)
	if distance <= maxDistance {
		*out = append(*out, node.indices...)
	}

	low := max(distance-maxDistance, 0)
	high := distance + maxDistance
	for childDistance, child := range node.children {
		if childDistance < low || childDistance > high {
			continue
		}
		searchNode(child, target, maxDistance, out)
	}
}

type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(size int) *unionFind {
	parent := make([]int, size)
	rank := make([]int, size)
	for i := range parent {
		parent[i] = i
	}
	return &unionFind{parent: parent, rank: rank}
}

func (u *unionFind) Find(x int) int {
	if u.parent[x] != x {
		u.parent[x] = u.Find(u.parent[x])
	}
	return u.parent[x]
}

func (u *unionFind) Union(a, b int) {
	rootA := u.Find(a)
	rootB := u.Find(b)
	if rootA == rootB {
		return
	}

	if u.rank[rootA] < u.rank[rootB] {
		u.parent[rootA] = rootB
		return
	}
	if u.rank[rootA] > u.rank[rootB] {
		u.parent[rootB] = rootA
		return
	}

	u.parent[rootB] = rootA
	u.rank[rootA]++
}

package util

import (
	"testing"
)

// testNode 用于测试的自定义树节点，模拟不同实体类型的 id/pid 字段命名
type testNode struct {
	ID       int64
	ParentID int64
	Name     string
	Children []*testNode
}

// newTestTreeBuilder 创建用于测试的 TreeBuilder
func newTestTreeBuilder() *TreeBuilder[*testNode] {
	return NewTreeBuilder[*testNode](
		func(n *testNode) int64 { return n.ID },
		func(n *testNode) int64 { return n.ParentID },
		0,
	)
}

// 构建测试数据：一棵 3 层树
//
//	root1 (id=1)
//	├── child1_1 (id=2)
//	│   └── grandchild1_1_1 (id=5)
//	└── child1_2 (id=3)
//	root2 (id=4)
//	└── child2_1 (id=6)
func buildTestFlatData() []*testNode {
	return []*testNode{
		{ID: 1, ParentID: 0, Name: "root1"},
		{ID: 2, ParentID: 1, Name: "child1_1"},
		{ID: 3, ParentID: 1, Name: "child1_2"},
		{ID: 4, ParentID: 0, Name: "root2"},
		{ID: 5, ParentID: 2, Name: "grandchild1_1_1"},
		{ID: 6, ParentID: 4, Name: "child2_1"},
	}
}

func TestBuildTree_Empty(t *testing.T) {
	builder := newTestTreeBuilder()
	result := builder.BuildTree(nil, func(n *testNode, children []*testNode) {
		n.Children = children
	})
	if result != nil {
		t.Errorf("期望 nil，实际 %v", result)
	}
}

func TestBuildTree_SingleRoot(t *testing.T) {
	builder := newTestTreeBuilder()
	data := []*testNode{
		{ID: 1, ParentID: 0, Name: "only_root"},
	}
	result := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})
	if len(result) != 1 {
		t.Fatalf("期望 1 个根节点，实际 %d", len(result))
	}
	if result[0].Name != "only_root" {
		t.Errorf("期望 Name=only_root，实际 %s", result[0].Name)
	}
	if len(result[0].Children) != 0 {
		t.Errorf("期望无子节点，实际 %d", len(result[0].Children))
	}
}

func TestBuildTree_MultiLevel(t *testing.T) {
	builder := newTestTreeBuilder()
	data := buildTestFlatData()

	result := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})

	// 应有 2 个根节点
	if len(result) != 2 {
		t.Fatalf("期望 2 个根节点，实际 %d", len(result))
	}

	// root1: 2 个子节点
	root1 := result[0]
	if root1.Name != "root1" {
		t.Errorf("期望第一个根节点 Name=root1，实际 %s", root1.Name)
	}
	if len(root1.Children) != 2 {
		t.Fatalf("期望 root1 有 2 个子节点，实际 %d", len(root1.Children))
	}

	// child1_1: 1 个子节点
	child1_1 := root1.Children[0]
	if child1_1.Name != "child1_1" {
		t.Errorf("期望 child1_1，实际 %s", child1_1.Name)
	}
	if len(child1_1.Children) != 1 {
		t.Fatalf("期望 child1_1 有 1 个子节点，实际 %d", len(child1_1.Children))
	}

	// grandchild1_1_1
	gc := child1_1.Children[0]
	if gc.Name != "grandchild1_1_1" {
		t.Errorf("期望 grandchild1_1_1，实际 %s", gc.Name)
	}
	if len(gc.Children) != 0 {
		t.Errorf("叶子节点不应有子节点")
	}

	// root2: 1 个子节点
	root2 := result[1]
	if root2.Name != "root2" {
		t.Errorf("期望第二个根节点 Name=root2，实际 %s", root2.Name)
	}
	if len(root2.Children) != 1 {
		t.Fatalf("期望 root2 有 1 个子节点，实际 %d", len(root2.Children))
	}
}

func TestBuildTree_DanglingChild(t *testing.T) {
	// pid 指向不存在的父节点，该节点不应出现在树中
	builder := newTestTreeBuilder()
	data := []*testNode{
		{ID: 1, ParentID: 0, Name: "root"},
		{ID: 2, ParentID: 99, Name: "orphan"},
	}

	result := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})

	if len(result) != 1 {
		t.Fatalf("期望 1 个根节点，实际 %d", len(result))
	}
	if len(result[0].Children) != 0 {
		t.Errorf("孤儿节点不应被挂载")
	}
}

func TestFlatten(t *testing.T) {
	builder := newTestTreeBuilder()
	data := buildTestFlatData()
	tree := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})

	flat := builder.Flatten(tree, func(n *testNode) []*testNode { return n.Children })

	if len(flat) != 6 {
		t.Fatalf("期望展平后 6 个节点，实际 %d", len(flat))
	}

	// 前序遍历顺序：root1 -> child1_1 -> grandchild1_1_1 -> child1_2 -> root2 -> child2_1
	expected := []string{"root1", "child1_1", "grandchild1_1_1", "child1_2", "root2", "child2_1"}
	for i, node := range flat {
		if node.Name != expected[i] {
			t.Errorf("位置 %d: 期望 %s，实际 %s", i, expected[i], node.Name)
		}
	}
}

func TestFlatten_Empty(t *testing.T) {
	builder := newTestTreeBuilder()
	flat := builder.Flatten(nil, func(n *testNode) []*testNode { return n.Children })
	if len(flat) != 0 {
		t.Errorf("期望空切片，实际 %d", len(flat))
	}
}

func TestFindNode_Found(t *testing.T) {
	builder := newTestTreeBuilder()
	data := buildTestFlatData()
	tree := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})

	// 查找根节点
	found, ok := builder.FindNode(tree, 1, func(n *testNode) []*testNode { return n.Children })
	if !ok || found.Name != "root1" {
		t.Errorf("期望找到 root1，实际 ok=%v found=%v", ok, found)
	}

	// 查找深层节点
	found, ok = builder.FindNode(tree, 5, func(n *testNode) []*testNode { return n.Children })
	if !ok || found.Name != "grandchild1_1_1" {
		t.Errorf("期望找到 grandchild1_1_1，实际 ok=%v found=%v", ok, found)
	}
}

func TestFindNode_NotFound(t *testing.T) {
	builder := newTestTreeBuilder()
	data := buildTestFlatData()
	tree := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})

	_, ok := builder.FindNode(tree, 999, func(n *testNode) []*testNode { return n.Children })
	if ok {
		t.Errorf("期望未找到，实际 ok=true")
	}
}

// TestBuildTree_CustomRootPID 验证自定义 rootPID 的适配能力
func TestBuildTree_CustomRootPID(t *testing.T) {
	type category struct {
		CID    int64
		Parent int64
		Sub    []*category
	}

	builder := NewTreeBuilder[*category](
		func(c *category) int64 { return c.CID },
		func(c *category) int64 { return c.Parent },
		-1, // rootPID = -1
	)

	data := []*category{
		{CID: 1, Parent: -1},
		{CID: 2, Parent: 1},
		{CID: 3, Parent: 1},
	}

	result := builder.BuildTree(data, func(c *category, sub []*category) {
		c.Sub = sub
	})

	if len(result) != 1 {
		t.Fatalf("期望 1 个根节点，实际 %d", len(result))
	}
	if len(result[0].Sub) != 2 {
		t.Errorf("期望 2 个子节点，实际 %d", len(result[0].Sub))
	}
}

// buildTestTree 辅助函数：构建测试用的树
func buildTestTree(t *testing.T) ([]*testNode, *TreeBuilder[*testNode]) {
	t.Helper()
	builder := newTestTreeBuilder()
	data := buildTestFlatData()
	tree := builder.BuildTree(data, func(n *testNode, children []*testNode) {
		n.Children = children
	})
	return tree, builder
}

func TestGetAncestors_RootNode(t *testing.T) {
	tree, builder := buildTestTree(t)
	ancestors := builder.GetAncestors(tree, 1, func(n *testNode) []*testNode { return n.Children })
	if len(ancestors) != 1 {
		t.Fatalf("根节点的祖先路径应仅包含自身，实际 %d 个", len(ancestors))
	}
	if ancestors[0].Name != "root1" {
		t.Errorf("期望 root1，实际 %s", ancestors[0].Name)
	}
}

func TestGetAncestors_DeepNode(t *testing.T) {
	tree, builder := buildTestTree(t)
	ancestors := builder.GetAncestors(tree, 5, func(n *testNode) []*testNode { return n.Children })

	expected := []string{"root1", "child1_1", "grandchild1_1_1"}
	if len(ancestors) != len(expected) {
		t.Fatalf("期望 %d 个祖先，实际 %d", len(expected), len(ancestors))
	}
	for i, node := range ancestors {
		if node.Name != expected[i] {
			t.Errorf("位置 %d: 期望 %s，实际 %s", i, expected[i], node.Name)
		}
	}
}

func TestGetAncestors_NotFound(t *testing.T) {
	tree, builder := buildTestTree(t)
	ancestors := builder.GetAncestors(tree, 999, func(n *testNode) []*testNode { return n.Children })
	if ancestors != nil {
		t.Errorf("不存在的节点应返回 nil，实际 %v", ancestors)
	}
}

func TestGetDescendants_LeafNode(t *testing.T) {
	tree, builder := buildTestTree(t)
	descendants := builder.GetDescendants(tree, 5, func(n *testNode) []*testNode { return n.Children })
	if descendants != nil {
		t.Errorf("叶子节点不应有后代，实际 %v", descendants)
	}
}

func TestGetDescendants_MiddleNode(t *testing.T) {
	tree, builder := buildTestTree(t)
	descendants := builder.GetDescendants(tree, 1, func(n *testNode) []*testNode { return n.Children })

	expected := []string{"child1_1", "grandchild1_1_1", "child1_2"}
	if len(descendants) != len(expected) {
		t.Fatalf("期望 %d 个后代，实际 %d", len(expected), len(descendants))
	}
	for i, node := range descendants {
		if node.Name != expected[i] {
			t.Errorf("位置 %d: 期望 %s，实际 %s", i, expected[i], node.Name)
		}
	}
}

func TestGetDescendants_NotFound(t *testing.T) {
	tree, builder := buildTestTree(t)
	descendants := builder.GetDescendants(tree, 999, func(n *testNode) []*testNode { return n.Children })
	if descendants != nil {
		t.Errorf("不存在的节点应返回 nil，实际 %v", descendants)
	}
}

func TestForEach(t *testing.T) {
	tree, builder := buildTestTree(t)
	visited := make([]string, 0)
	builder.ForEach(tree, func(n *testNode) {
		visited = append(visited, n.Name)
	}, func(n *testNode) []*testNode { return n.Children })

	expected := []string{"root1", "child1_1", "grandchild1_1_1", "child1_2", "root2", "child2_1"}
	if len(visited) != len(expected) {
		t.Fatalf("期望遍历 %d 个节点，实际 %d", len(expected), len(visited))
	}
	for i, name := range visited {
		if name != expected[i] {
			t.Errorf("位置 %d: 期望 %s，实际 %s", i, expected[i], name)
		}
	}
}

func TestFilter(t *testing.T) {
	tree, builder := buildTestTree(t)

	// 只保留 id=5 的节点，祖先路径应保留
	filtered := builder.Filter(tree,
		func(n *testNode) bool { return n.ID == 5 },
		func(n *testNode) []*testNode { return n.Children },
		func(n *testNode, children []*testNode) { n.Children = children },
	)

	if len(filtered) != 1 {
		t.Fatalf("期望 1 棵树，实际 %d", len(filtered))
	}
	// root1 -> child1_1 -> grandchild1_1_1
	if filtered[0].Name != "root1" {
		t.Errorf("期望根节点 root1 被保留，实际 %s", filtered[0].Name)
	}
	if len(filtered[0].Children) != 1 {
		t.Fatalf("期望 root1 保留 1 个子节点，实际 %d", len(filtered[0].Children))
	}
	if filtered[0].Children[0].Name != "child1_1" {
		t.Errorf("期望 child1_1 被保留，实际 %s", filtered[0].Children[0].Name)
	}
}

func TestFilter_NoneMatch(t *testing.T) {
	tree, builder := buildTestTree(t)
	filtered := builder.Filter(tree,
		func(n *testNode) bool { return n.ID == 999 },
		func(n *testNode) []*testNode { return n.Children },
		func(n *testNode, children []*testNode) { n.Children = children },
	)
	if len(filtered) != 0 {
		t.Errorf("无匹配时期望空切片，实际 %d", len(filtered))
	}
}

func TestCountNodes(t *testing.T) {
	tree, builder := buildTestTree(t)
	count := builder.CountNodes(tree, func(n *testNode) []*testNode { return n.Children })
	if count != 6 {
		t.Errorf("期望 6 个节点，实际 %d", count)
	}
}

func TestCountNodes_Empty(t *testing.T) {
	builder := newTestTreeBuilder()
	count := builder.CountNodes(nil, func(n *testNode) []*testNode { return n.Children })
	if count != 0 {
		t.Errorf("期望 0 个节点，实际 %d", count)
	}
}

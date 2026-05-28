package util

// TreeBuilder 通过函数式提取器支持任意类型的树操作。
// T 应为指针类型（如 *sdkdto.TaskTreeDTO），不要求实体实现特定接口，
// 通过闭包提取 id/pid，实现对任意类型的零侵入适配。
type TreeBuilder[T any] struct {
	getID   func(T) int64
	getPID  func(T) int64
	rootPID int64 // 根节点的 pid 值（通常为 0）
}

// NewTreeBuilder 创建一个可复用的树构建器。
//   - getID: 从节点提取唯一标识
//   - getPID: 从节点提取父节点标识
//   - rootPID: 根节点的 pid 值，pid 等于此值的节点被视为根节点
func NewTreeBuilder[T any](getID func(T) int64, getPID func(T) int64, rootPID int64) *TreeBuilder[T] {
	return &TreeBuilder[T]{
		getID:   getID,
		getPID:  getPID,
		rootPID: rootPID,
	}
}

// BuildTree 将平坦列表转换为树形结构，O(n) 算法。
// setChildren 用于将子节点列表挂载到父节点上（因为不同类型的 children 字段名不同）。
// 返回根节点列表。
func (b *TreeBuilder[T]) BuildTree(nodes []T, setChildren func(T, []T)) []T {
	if len(nodes) == 0 {
		return nil
	}

	nodeMap := make(map[int64]T, len(nodes))
	childrenMap := make(map[int64][]T)
	roots := make([]T, 0)

	for _, node := range nodes {
		id := b.getID(node)
		pid := b.getPID(node)
		nodeMap[id] = node
		setChildren(node, nil)

		if pid == b.rootPID {
			roots = append(roots, node)
		} else {
			childrenMap[pid] = append(childrenMap[pid], node)
		}
	}

	for pid, children := range childrenMap {
		if parent, ok := nodeMap[pid]; ok {
			setChildren(parent, children)
		}
	}

	return roots
}

// Flatten 将树形结构展平为平坦列表（前序深度优先遍历）。
func (b *TreeBuilder[T]) Flatten(roots []T, getChildren func(T) []T) []T {
	result := make([]T, 0, len(roots))
	b.forEachNode(roots, getChildren, func(node T) {
		result = append(result, node)
	})
	return result
}

// FindNode 在树中按 ID 查找节点。返回 (node, true) 表示找到，(zero, false) 表示未找到。
func (b *TreeBuilder[T]) FindNode(roots []T, id int64, getChildren func(T) []T) (T, bool) {
	for _, root := range roots {
		if b.getID(root) == id {
			return root, true
		}
		children := getChildren(root)
		if len(children) > 0 {
			if found, ok := b.FindNode(children, id, getChildren); ok {
				return found, true
			}
		}
	}
	var zero T
	return zero, false
}

// forEachNode 深度优先遍历内部辅助函数。
func (b *TreeBuilder[T]) forEachNode(roots []T, getChildren func(T) []T, callback func(T)) {
	for _, node := range roots {
		callback(node)
		children := getChildren(node)
		if len(children) > 0 {
			b.forEachNode(children, getChildren, callback)
		}
	}
}

// GetAncestors 获取从根到目标节点的祖先路径（包含目标节点自身）。
// 未找到目标节点时返回 nil。
func (b *TreeBuilder[T]) GetAncestors(roots []T, id int64, getChildren func(T) []T) []T {
	parentMap := make(map[int64]int64) // childID -> parentID
	b.buildParentMap(roots, getChildren, parentMap)

	// 检查目标节点是否存在
	if _, exists := parentMap[id]; !exists {
		// 可能是根节点本身
		for _, root := range roots {
			if b.getID(root) == id {
				return []T{root}
			}
		}
		return nil
	}

	// 回溯构建路径
	path := make([]int64, 0)
	current := id
	for {
		path = append(path, current)
		parentID, exists := parentMap[current]
		if !exists {
			break
		}
		current = parentID
	}

	// 收集路径上的节点
	nodeMap := make(map[int64]T)
	b.forEachNode(roots, getChildren, func(node T) {
		nodeMap[b.getID(node)] = node
	})

	result := make([]T, 0, len(path))
	for i := len(path) - 1; i >= 0; i-- {
		if node, ok := nodeMap[path[i]]; ok {
			result = append(result, node)
		}
	}
	return result
}

// buildParentMap 构建子节点到父节点的映射。
func (b *TreeBuilder[T]) buildParentMap(roots []T, getChildren func(T) []T, parentMap map[int64]int64) {
	for _, node := range roots {
		id := b.getID(node)
		children := getChildren(node)
		for _, child := range children {
			parentMap[b.getID(child)] = id
		}
		if len(children) > 0 {
			b.buildParentMap(children, getChildren, parentMap)
		}
	}
}

// GetDescendants 获取目标节点的所有后代（不含目标节点自身）。
// 未找到目标节点时返回 nil。
func (b *TreeBuilder[T]) GetDescendants(roots []T, id int64, getChildren func(T) []T) []T {
	target, ok := b.FindNode(roots, id, getChildren)
	if !ok {
		return nil
	}
	children := getChildren(target)
	if len(children) == 0 {
		return nil
	}
	return b.Flatten(children, getChildren)
}

// ForEach 对树中所有节点执行深度优先遍历回调。
func (b *TreeBuilder[T]) ForEach(roots []T, callback func(T), getChildren func(T) []T) {
	b.forEachNode(roots, getChildren, callback)
}

// Filter 过滤树：保留匹配 predicate 的节点，同时保留通往匹配节点的祖先路径。
// setChildren 用于重新构建过滤后的子节点列表。
func (b *TreeBuilder[T]) Filter(roots []T, predicate func(T) bool, getChildren func(T) []T, setChildren func(T, []T)) []T {
	result := make([]T, 0)
	for _, node := range roots {
		filteredChildren := b.Filter(getChildren(node), predicate, getChildren, setChildren)
		if predicate(node) || len(filteredChildren) > 0 {
			setChildren(node, filteredChildren)
			result = append(result, node)
		}
	}
	return result
}

// CountNodes 统计树中节点总数。
func (b *TreeBuilder[T]) CountNodes(roots []T, getChildren func(T) []T) int {
	count := 0
	b.forEachNode(roots, getChildren, func(_ T) { count++ })
	return count
}

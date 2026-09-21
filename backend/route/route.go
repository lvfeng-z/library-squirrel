// Package route 是插件触发式功能的多候选路由基座：候选排序、逐候选尝试、
// 失败二分分发与收口两态判定四件不变量在此单点实现，候选发现与调用协议由各消费面
// 经 Adapter 接入。
package route

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Outcome 单候选调用结果三态，由适配器判定后交由基座分发。
type Outcome int

const (
	// NotApplicable 候选明示该触发不归自己处理：顺延下一候选。
	NotApplicable Outcome = iota
	// Failed 候选真失败：终止整次路由并点名报错。
	Failed
	// Succeeded 候选成功：以该候选为本次路由终点。
	Succeeded
)

// ErrNoCandidates 发现机制零产出。触发键（URL / siteKey 等）的点名由调用方包装。
var ErrNoCandidates = errors.New("无候选可路由")

// AllNotApplicableError 候选全部声明不归属。Candidates 为已试候选的点名清单，按尝试序。
type AllNotApplicableError struct {
	Candidates []string
}

func (e *AllNotApplicableError) Error() string {
	return fmt.Sprintf("候选均声明不归属本次触发：%s", strings.Join(e.Candidates, "、"))
}

// Adapter 自定路由接入：候选发现、排序键、点名与调用三态分类。
type Adapter[C any, R any] interface {
	// Candidates 发现候选。显选候选（若有）由实现在此置首，基座不感知显选概念。
	Candidates(ctx context.Context) ([]C, error)
	// OrderKey 候选的复合全键（候选集内唯一），基座按其字典序稳定排序。
	OrderKey(c C) string
	// Describe 候选点名，用于失败报错与全不适配清单列名。
	Describe(c C) string
	// Invoke 调用候选并给出三态分类；Outcome 为 Failed 时应携带失败原因。
	Invoke(ctx context.Context, c C) (R, Outcome, error)
}

// Route 按 OrderKey 字典序逐候选尝试：不适配者顺延、真失败即终止并点名、首个成功者即终点。
// 收口两态分报——零候选返回 ErrNoCandidates，候选全部不适配返回 *AllNotApplicableError。
func Route[C any, R any](ctx context.Context, a Adapter[C, R]) (R, error) {
	var zero R

	candidates, err := a.Candidates(ctx)
	if err != nil {
		return zero, err
	}
	if len(candidates) == 0 {
		return zero, ErrNoCandidates
	}

	ordered := make([]C, len(candidates))
	copy(ordered, candidates)
	// 稳定排序：OrderKey 相同的候选保持适配器给出的相对序。
	sort.SliceStable(ordered, func(i, j int) bool {
		return a.OrderKey(ordered[i]) < a.OrderKey(ordered[j])
	})

	notApplicable := make([]string, 0, len(ordered))
	for _, c := range ordered {
		result, outcome, invokeErr := a.Invoke(ctx, c)
		switch outcome {
		case Succeeded:
			return result, nil
		case NotApplicable:
			notApplicable = append(notApplicable, a.Describe(c))
		default:
			// Failed 与未定义态一律按真失败终止，不静默顺延。
			if invokeErr == nil {
				invokeErr = errors.New("未提供失败原因")
			}
			return zero, fmt.Errorf("候选 %s 处理失败: %w", a.Describe(c), invokeErr)
		}
	}

	return zero, &AllNotApplicableError{Candidates: notApplicable}
}

package settingresolver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// DefaultEvalTimeout 求值超时定值：脚本求值超过该时长即向运行时注入中断。
const DefaultEvalTimeout = 500 * time.Millisecond

// DefaultScriptSizeLimit 脚本体积上限：超过即拒收，不进入编译与求值。
const DefaultScriptSizeLimit = 64 * 1024

// scriptDisplayName 编译产物在错误信息中的来源名（各插件清单声明的脚本文件名）。
const scriptDisplayName = "resolver.js"

// 中断原因值：定时器与取消监听各自注入的哨兵，供中断错误回读分类。
// 超时按 FailureTimeout 分类；调用方取消原样返回 context 的取消错误（求值被放弃，
// 不属于脚本失败）。
var (
	errTimeoutInterrupt  = errors.New("resolver 求值超时")
	errCanceledInterrupt = errors.New("resolver 求值被调用方取消")
)

// Runner goja 适配器：同一脚本内容编译一次为 Program 复用（编译缓存），每次求值
// 新建零绑定的受限运行时。编译缓存按脚本内容寻址，无需失效——同内容脚本复用
// 同一 Program，不同内容互不干扰。
type Runner struct {
	evalTimeout     time.Duration
	scriptSizeLimit int

	mu       sync.Mutex
	programs map[string]*goja.Program
}

// NewRunner 创建使用默认超时与体积上限的运行器。
func NewRunner() *Runner {
	return &Runner{
		evalTimeout:     DefaultEvalTimeout,
		scriptSizeLimit: DefaultScriptSizeLimit,
		programs:        make(map[string]*goja.Program),
	}
}

// Evaluate 编译（带缓存）并求值脚本，校验返回值形状。失败返回 *EvalError（调用方
// 取消时返回 context 的取消错误）；条目级不合法不构成失败，记入 Result.Rejected。
func (r *Runner) Evaluate(ctx context.Context, script string, in Input) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(script) > r.scriptSizeLimit {
		return nil, &EvalError{
			Kind:    FailureScriptTooLarge,
			Message: fmt.Sprintf("脚本 %d 字节超过体积上限 %d 字节", len(script), r.scriptSizeLimit),
		}
	}
	prog, err := r.compiledProgram(script)
	if err != nil {
		return nil, err
	}

	vm := newRestrictedRuntime()
	guard := armInterrupt(ctx, vm, r.evalTimeout)
	defer guard.stop()

	// 顶层代码（声明入口函数等）与入口调用都在中断窗口内
	if _, err := vm.RunProgram(prog); err != nil {
		return nil, classifyEvalError(ctx, err)
	}
	resolve, ok := goja.AssertFunction(vm.Get(EntryFuncName))
	if !ok {
		return nil, &EvalError{
			Kind:    FailureRuntime,
			Message: fmt.Sprintf("脚本未定义可调用的入口函数 %s", EntryFuncName),
		}
	}
	ret, err := resolve(goja.Undefined(), vm.ToValue(in.toJSInput()))
	if err != nil {
		return nil, classifyEvalError(ctx, err)
	}
	return ValidateOutput(ret.Export())
}

// toJSInput 构造与契约输入形状一致的值（settings 键承载设置快照），并深拷贝
// JSON 形状的值——脚本对输入的改写不泄回调用方数据。空设置图映射为空对象。
func (in Input) toJSInput() map[string]interface{} {
	settings := copyJSONShape(in.Settings)
	if settings == nil {
		settings = map[string]interface{}{}
	}
	return map[string]interface{}{"settings": settings}
}

// copyJSONShape 深拷贝 JSON 形状的值（对象与数组递归复制），标量与未知形态原样
// 返回（标量不可变；未知形态超出契约输入范围）。
func copyJSONShape(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		cp := make(map[string]interface{}, len(val))
		for k, item := range val {
			cp[k] = copyJSONShape(item)
		}
		return cp
	case []interface{}:
		cp := make([]interface{}, len(val))
		for i, item := range val {
			cp[i] = copyJSONShape(item)
		}
		return cp
	default:
		return v
	}
}

// newRestrictedRuntime 新建受限运行时：删除 Date 与 Math.random 两个非确定性源，
// 其余保持引擎内建；不注入任何宿主对象（无 I/O、网络、文件、计时器）。
func newRestrictedRuntime() *goja.Runtime {
	vm := goja.New()
	vm.GlobalObject().Delete("Date")
	if math, ok := vm.Get("Math").(*goja.Object); ok {
		_ = math.Delete("random")
	}
	return vm
}

// interruptGuard 汇聚一次求值的两个中断源（超时定时器与调用方取消监听），
// 求值结束后统一释放。
type interruptGuard struct {
	timer     *time.Timer
	watchDone chan struct{}
}

func (g interruptGuard) stop() {
	if g.timer != nil {
		g.timer.Stop()
	}
	close(g.watchDone)
}

// armInterrupt 布防两个中断源：超时定时器取「超时定值」与「调用方截止时间」中
// 较早到点者；另有监听协程在调用方取消（含无截止时间的提前取消）时立即中断。
// 截止时间已过时直接以取消哨兵预置中断，下一次 Run* 立即中断返回。
func armInterrupt(ctx context.Context, vm *goja.Runtime, evalTimeout time.Duration) interruptGuard {
	g := interruptGuard{watchDone: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt(errCanceledInterrupt)
		case <-g.watchDone:
		}
	}()

	delay := evalTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if until := time.Until(deadline); until <= 0 {
			vm.Interrupt(errCanceledInterrupt)
			return g
		} else if until < delay {
			delay = until
		}
	}
	g.timer = time.AfterFunc(delay, func() { vm.Interrupt(errTimeoutInterrupt) })
	return g
}

// compiledProgram 返回脚本内容的预编译 Program：首次调用编译并缓存，语法错归
// FailureSyntax（不缓存失败产物）。
func (r *Runner) compiledProgram(script string) (*goja.Program, error) {
	r.mu.Lock()
	prog, ok := r.programs[script]
	r.mu.Unlock()
	if ok {
		return prog, nil
	}

	prog, err := goja.Compile(scriptDisplayName, script, false)
	if err != nil {
		return nil, &EvalError{Kind: FailureSyntax, Message: err.Error(), Err: err}
	}

	r.mu.Lock()
	r.programs[script] = prog
	r.mu.Unlock()
	return prog, nil
}

// classifyEvalError 将求值期错误归入失败分类：中断错误以 *goja.InterruptedError
// 类型断言识别、回读注入原因区分超时与调用方取消（取消原样返回 context 的取消
// 错误）；其余错误（含脚本抛出的 JS 异常）归 FailureRuntime。
func classifyEvalError(ctx context.Context, err error) error {
	var intErr *goja.InterruptedError
	if errors.As(err, &intErr) {
		if intErr.Value() == interface{}(errCanceledInterrupt) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return context.Canceled
		}
		return &EvalError{Kind: FailureTimeout, Message: errTimeoutInterrupt.Error(), Err: err}
	}
	return &EvalError{Kind: FailureRuntime, Message: err.Error(), Err: err}
}

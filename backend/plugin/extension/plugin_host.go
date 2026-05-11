package extension

import (
	"encoding/json"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"go.uber.org/zap"
)

// registerHostHandlers 注册主进程侧 RPC 处理函数
// 处理插件通过 ctx/* 方法发起的请求，委托给 PluginContext 或创建代理
func registerHostHandlers(handlers map[string]pluginsdk.Handler, process *PluginProcess) {
	// --- 扩展点注册 ---
	handlers["ctx/registerTaskHandler"] = func(params json.RawMessage) (any, error) {
		var p struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}

		proxy := &TaskHandlerProxy{
			process:       process,
			contributionId: p.ID,
		}

		metadata := model.ExtensionMetadata{
			Type:           model.ExtensionTypeTaskHandler,
			ID:             p.ID,
			PluginID:       process.pluginInfo.ID,
			PluginPublicID: process.pluginInfo.PublicID,
			Name:           p.Name,
			Description:    p.Description,
		}
		var handler pluginsdk.TaskHandler = proxy
		if err := process.taskHandlerRegistry.Register(model.NewExtension(metadata, handler)); err != nil {
			return nil, err
		}
		logger.Log.Info("TaskHandler 已注册（子进程模式）",
			zap.String("plugin", process.pluginInfo.PublicID), zap.String("id", p.ID))
		return nil, nil
	}

	handlers["ctx/registerSiteBrowser"] = func(params json.RawMessage) (any, error) {
		var p struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}

		proxy := &SiteBrowserProxy{
			process:       process,
			contributionId: p.ID,
		}

		metadata := model.ExtensionMetadata{
			Type:           model.ExtensionTypeSiteBrowser,
			ID:             p.ID,
			PluginID:       process.pluginInfo.ID,
			PluginPublicID: process.pluginInfo.PublicID,
			Name:           p.Name,
			Description:    p.Description,
		}
		var browser pluginsdk.SiteBrowser = proxy
		if err := process.siteBrowserRegistry.Register(model.NewExtension(metadata, browser)); err != nil {
			return nil, err
		}
		logger.Log.Info("SiteBrowser 已注册（子进程模式）",
			zap.String("plugin", process.pluginInfo.PublicID), zap.String("id", p.ID))
		return nil, nil
	}

	handlers["ctx/unregisterSiteBrowser"] = func(params json.RawMessage) (any, error) {
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return nil, process.siteBrowserRegistry.Unregister(process.pluginInfo.PublicID, p.ID)
	}

	// --- 插件数据持久化 ---
	handlers["ctx/getPluginData"] = func(params json.RawMessage) (any, error) {
		data, err := process.pluginCtx.GetPluginData()
		if err != nil {
			return nil, err
		}
		return map[string]string{"data": data}, nil
	}

	handlers["ctx/setPluginData"] = func(params json.RawMessage) (any, error) {
		var p struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return nil, process.pluginCtx.SetPluginData(p.Data)
	}

	// --- 加密存储 ---
	handlers["ctx/storeEncryptedValue"] = func(params json.RawMessage) (any, error) {
		var p struct {
			PlainValue   string `json:"plainValue"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		key, err := process.pluginCtx.StoreEncryptedValue(p.PlainValue, p.Description)
		if err != nil {
			return nil, err
		}
		return map[string]string{"key": key}, nil
	}

	handlers["ctx/getDecryptedValue"] = func(params json.RawMessage) (any, error) {
		var p struct {
			StorageKey string `json:"storageKey"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		value, err := process.pluginCtx.GetDecryptedValue(p.StorageKey)
		if err != nil {
			return nil, err
		}
		return map[string]string{"value": value}, nil
	}

	handlers["ctx/removeEncryptedValue"] = func(params json.RawMessage) (any, error) {
		var p struct {
			StorageKey string `json:"storageKey"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return nil, process.pluginCtx.RemoveEncryptedValue(p.StorageKey)
	}

	// --- 业务查询 ---
	handlers["ctx/getWorkSetBySiteWorkSetId"] = func(params json.RawMessage) (any, error) {
		var p struct {
			SiteWorkSetID string `json:"siteWorkSetId"`
			SiteName      string `json:"siteName"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return process.pluginCtx.GetWorkSetBySiteWorkSetId(p.SiteWorkSetID, p.SiteName)
	}

	handlers["ctx/addSite"] = func(params json.RawMessage) (any, error) {
		var p struct {
			Sites []*pluginsdk.Site `json:"sites"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return nil, process.pluginCtx.AddSite(p.Sites)
	}

	// --- 任务 ---
	handlers["ctx/registerUrlListener"] = func(params json.RawMessage) (any, error) {
		var p struct {
			ContributionID string   `json:"contributionId"`
			Patterns       []string `json:"patterns"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return nil, process.pluginCtx.RegisterUrlListener(p.ContributionID, p.Patterns)
	}

	handlers["ctx/unregisterUrlListener"] = func(params json.RawMessage) (any, error) {
		return nil, process.pluginCtx.UnregisterUrlListener()
	}

	handlers["ctx/createTask"] = func(params json.RawMessage) (any, error) {
		var p struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return process.pluginCtx.CreateTask(p.URL)
	}

	// --- 路径 ---
	handlers["ctx/getPluginRoot"] = func(params json.RawMessage) (any, error) {
		var p struct {
			IsRelative bool `json:"isRelative"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		path := process.pluginCtx.GetPluginRoot(p.IsRelative)
		return map[string]string{"path": path}, nil
	}

	// --- 日志 ---
	// 日志处理器：读取 loggerName 字段，使用对应的 Named 子 logger 输出
	pc := process.pluginCtx.(*pluginContext)

	handlers["ctx/infof"] = func(params json.RawMessage) (any, error) {
		var p struct {
			Template   string `json:"template"`
			Args       []any  `json:"args"`
			LoggerName string `json:"loggerName,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, nil
		}
		pc.ResolveLogger(p.LoggerName).Infof(p.Template, p.Args...)
		return nil, nil
	}

	handlers["ctx/debugf"] = func(params json.RawMessage) (any, error) {
		var p struct {
			Template   string `json:"template"`
			Args       []any  `json:"args"`
			LoggerName string `json:"loggerName,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, nil
		}
		pc.ResolveLogger(p.LoggerName).Debugf(p.Template, p.Args...)
		return nil, nil
	}

	handlers["ctx/warnf"] = func(params json.RawMessage) (any, error) {
		var p struct {
			Template   string `json:"template"`
			Args       []any  `json:"args"`
			LoggerName string `json:"loggerName,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, nil
		}
		pc.ResolveLogger(p.LoggerName).Warnf(p.Template, p.Args...)
		return nil, nil
	}

	handlers["ctx/errorf"] = func(params json.RawMessage) (any, error) {
		var p struct {
			Template   string `json:"template"`
			Args       []any  `json:"args"`
			LoggerName string `json:"loggerName,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, nil
		}
		pc.ResolveLogger(p.LoggerName).Errorf(p.Template, p.Args...)
		return nil, nil
	}
}

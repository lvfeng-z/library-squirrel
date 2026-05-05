package main

import (
	"embed"
	"time"

	"github.com/library-squirrel/wails/backend/base/logger"
	"github.com/library-squirrel/wails/backend/database"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	application.RegisterEvent[string]("time")
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {
	// 初始化日志（最优先，确保后续所有代码都可使用 logger.Log）
	if err := logger.Init(); err != nil {
		panic("Failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	// Create App instance
	app, err := NewApp()
	if err != nil {
		logger.Log.Fatalf("Failed to create app: %v", err)
	}

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	wailsApp := application.New(application.Options{
		Name:        "library-squirrel",
		Description: "A personal resource library with tag-based search",
		Services: []application.Service{
			application.NewService(app.LocalTagHandler),
			application.NewService(app.LocalAuthorHandler),
			application.NewService(app.SiteTagHandler),
			application.NewService(app.SiteAuthorHandler),
			application.NewService(app.SiteHandler),
			application.NewService(app.ResourceHandler),
			application.NewService(app.WorkHandler),
			application.NewService(app.WorkSetHandler),
			application.NewService(app.SearchHandler),
			application.NewService(app.SettingsHandler),
			application.NewService(app.SecureStorageHandler),
			application.NewService(app.BackupHandler),
			application.NewService(app.AppLauncherHandler),
			application.NewService(app.FileSysUtilHandler),
			application.NewService(app.PluginHandler),
			application.NewService(app.TaskHandler),
			application.NewService(app.TaskManagerHandler),
			application.NewService(app.SlotHandler),
			application.NewService(app.SiteBrowserHandler),
			application.NewService(app.ReWorkAuthorHandler),
			application.NewService(app.ReWorkTagHandler),
			application.NewService(app.PluginTaskUrlListenerHandler),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Library squirrel",
		Width:            1280,
		Height:           720,
		MinWidth:         800,
		MinHeight:        450,
		BackgroundColour: application.NewRGB(217, 217, 217),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	// Set the Wails event emitter for slot pusher
	app.SetEventEmitter(wailsApp.Event)

	// Register window event callbacks
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(e *application.WindowEvent) {
		app.onDomReady()
	})

	window.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !app.onBeforeClose() {
			// Allow close - but we need to call window.Close() explicitly in Wails
			// The event.Cancel() prevents the close
		} else {
			// Cancel the close
			e.Cancel()
		}
	})

	// Create a goroutine that emits an event containing the current time every second.
	// The frontend can listen to this event and update the UI accordingly.
	go func() {
		for {
			now := time.Now().Format(time.RFC1123)
			wailsApp.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()

	// Run the application. This blocks until the application has been exited.
	err = wailsApp.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		logger.Log.Fatal(err)
	}

	// 关闭数据库连接
	err = database.Close()
	if err != nil {
		logger.Log.Fatal(err)
		return
	}
}

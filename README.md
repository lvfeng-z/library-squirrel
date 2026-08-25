# Welcome to Your New Wails3 Project!

Congratulations on generating your Wails3 application! This README will guide you through the next steps to get your project up and running.

## Getting Started

1. Navigate to your project directory in the terminal.

2. To run your application in development mode, use the following command:

   ```
   wails3 dev
   ```

   This will start your application and enable hot-reloading for both frontend and backend changes.

3. To build your application for production, use:

   ```
   wails3 build
   ```

   This will create a production-ready executable in the `build` directory.

## Exploring Wails3 Features

Now that you have your project set up, it's time to explore the features that Wails3 offers:

1. **Check out the examples**: The best way to learn is by example. Visit the `examples` directory in the `v3/examples` directory to see various sample applications.

2. **Run an example**: To run any of the examples, navigate to the example's directory and use:

   ```
   go run .
   ```

   Note: Some examples may be under development during the alpha phase.

3. **Explore the documentation**: Visit the [Wails3 documentation](https://v3.wails.io/) for in-depth guides and API references.

4. **Join the community**: Have questions or want to share your progress? Join the [Wails Discord](https://discord.gg/JDdSxwjhGf) or visit the [Wails discussions on GitHub](https://github.com/wailsapp/wails/discussions).

## Project Structure

Take a moment to familiarize yourself with your project structure:

- `frontend/`: Contains your frontend code (HTML, CSS, JavaScript/TypeScript)
- `main.go`: The entry point of your Go backend
- `app.go`: Define your application structure and methods here
- `wails.json`: Configuration file for your Wails project

## Next Steps

1. Modify the frontend in the `frontend/` directory to create your desired UI.
2. Add backend functionality in `main.go`.
3. Use `wails3 dev` to see your changes in real-time.
4. When ready, build your application with `wails3 build`.

Happy coding with Wails3! If you encounter any issues or have questions, don't hesitate to consult the documentation or reach out to the Wails community.

## 许可协议

本项目采用 GPL-3.0-or-later（GNU 通用公共许可证第 3 版或其后续版本）授权，全文见 [LICENSE](LICENSE)。

Copyright (C) 2026 lvfeng

本程序为自由软件：你可以依据自由软件基金会发布的 GNU 通用公共许可证（第 3 版或任意后续版本，任选其一）重新发布或修改它。发布本程序是希望它有用，但不提供任何担保——不含适销性或特定用途适用性担保。具体条款详见 [LICENSE](LICENSE)。

## 第三方插件声明

本软件采用开放插件架构，支持安装第三方插件。第三方插件由其作者独立开发与维护，用户与插件作者之间就插件产生的一切交易（含付费）与权益关系，均发生在用户与插件作者之间，与本软件作者无关；本软件对第三方插件不作任何担保。第三方插件的问题请向其作者反馈。

插件管理页中的来源与信任标记用于区分官方捆绑插件与第三方安装的插件。

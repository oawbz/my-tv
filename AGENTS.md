# 项目约定

## Android 构建

- 本项目统一使用 H255 远程 Docker 环境构建 Android 版本，不依赖本机 Android SDK/NDK。
- 在项目根目录执行 `./build-android-docker.sh assembleDebug`。脚本使用 `ghcr.io/cirruslabs/android-sdk:34-ndk`，并传入 `-PIS_SO_BUILD=false`，以使用仓库中的预编译 native 库。
- Docker 容器会复用 H255 上的持久缓存目录 `$HOME/.cache/my-tv/android-sdk`，NDK 和 CMake 安装包在容器退出后仍保留；SDK 管理器和许可证继续使用 Android 镜像自带文件。需要更换缓存目录时，可在运行脚本前设置 `ANDROID_SDK_CACHE`。
- 将待构建源码同步到 H255 工作目录 `/tmp/my-tv-d65ac508` 后再执行构建脚本。远程设备名称为 `AMD H255`；通过项目已配置的 MCP/SSH 连接访问，不要把连接凭据写入文档或提交到仓库。
- Debug APK 生成在远程路径 `/tmp/my-tv-d65ac508/app/build/outputs/apk/debug/app-debug.apk`。
- 若依赖解析或 Android SDK 工具下载暂时失败，先判断是否为远程网络或镜像问题，再重试。遇到 Kotlin 或 CMake 源码错误时先修复，再在 H255 重建。
- 除非任务明确要求，不为解决构建问题升级 Gradle 或项目依赖。

# 我的电视

Android TV 直播播放器。启动时从 `https://awbz.cn/tv/channels.json` 获取频道、台标地址和播放地址；台标与视频由对应远程地址加载。应用不内置频道列表，也不保存频道或台标的磁盘缓存。断网时无法加载频道或播放。

## 频道配置

远程 JSON 使用 `version: 1` 和非空的 `channels` 数组。每个频道包含 `name`、`group`、非空的 `urls` 数组和 `logo`，播放地址和台标地址均为 HTTP 或 HTTPS URL。发布前运行 `python3 scripts/validate_channels.py channels.json`，部署后运行 `python3 scripts/validate_channels.py https://awbz.cn/tv/channels.json` 验证线上内容。单条坏频道会在运行时跳过；全部不可用时应用提示重试或退出。

## 构建与发布

Android APK 仅在 AMD H255 远程设备的 Docker 环境构建。将源码同步到 `/tmp/my-tv-d65ac508` 后，在该目录执行 `./build-android-docker.sh assembleDebug`，产物为 `app/build/outputs/apk/debug/app-debug.apk`。发布版也应在该环境构建、签名并完成电视设备播放验收后手动发布。GitHub Actions 目前只校验频道配置，不自动构建或发布 APK。

## 使用

1. 在 H255 构建 APK，并在电视设备上验证频道加载、播放与换台。
2. 安装
    * U盘安装
    * 小米电视可以使用小米电视助手进行安装
    * 如电视可以启用ADB，也可以通过ADB进行安装
       ```shell
       adb install my-tv.apk
       ```

![image](./screenshots/img_3.png)
![image](./screenshots/img_2.png)
![image](./screenshots/img_1.png)

## 更新日志

[更新日志](./HISTORY.md)

## TODO

* 音量不同
* 大湾区卫视、广东4k超高清、广东珠江、三沙卫视
* CHC高清三个电影频道
* 地方频道
* 收藏夹
* 海外
* 隐藏频道
* 亮度调节
* 音量调节
* 軟解
* 自動更新

無法自啟的設備：
斐讯N1盒子，[Phicomm] Phicomm p230 (Android 7.1.2)

閃退：
中国移动盒子(新魔百和M302A) 4.4.2

## 版权说明

[LICENSE](./LICENSE)

本项目仅供学习研究，禁止用于商业用途，请于下载二十四小时内删除。

本项目可能随时终止，请大家谨慎使用，建议使用官方渠道进行观看。

本项目使用的部分代码、图片、文字等资源来源于网络，如有侵权，请联系删除。

## 赞赏

![image](./screenshots/appreciate.jpeg)

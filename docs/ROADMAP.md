# Skillsync 产品升级方案（参考 CleanMyMac 设计）

## P0 基础修复
- [x] 端口握手：app 用 `--port 0` 启动服务，服务打印 `LISTENING <port>`，app 读 stdout 获取真实端口，杜绝旧进程残留/端口冲突
- [x] 前端错误可见：API 失败显示红色错误横幅而非卡"加载中"；每 30s 自动刷新
- [x] 应用图标：程序化生成 1024px 渐变图标 → iconutil 转 icns，Dock/标题栏显示

## P1 信息架构（CleanMyMac 式侧边栏多页）
- [x] 总览：状态横幅 + 大数字卡片（变更/落后超前/待处理/上次同步时间）+ 一键同步
- [x] Skills 页：全部 skill 列表（名称、描述=SKILL.md frontmatter、最后修改时间）、搜索过滤
- [x] 待对账页：pending 报告 + 双边修改冲突，一键选边
- [x] 同步日志页：skillsync.log 时间线
- [x] 设置页：同步间隔、仓库地址、pre-sync 闸开关、开机自启（launchd 安装/卸载）、主题

## P2 体验
- [x] 实时同步进度：流式日志（fetch stream），同步中逐行显示
- [x] 上次同步时间/结果持久化（state/last-sync.json）
- [ ] 首次运行向导（无配置时引导填仓库地址）
- [ ] 菜单栏托盘与窗口 app 合并（Dock + 菜单栏双形态）

## 发布
- v0.2.0：P0+P1+P2 核心，macOS 窗口 app + CLI + 托盘

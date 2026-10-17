# Windows 免安装版

完整解压 `artex-windows-amd64-portable.zip`，双击 `start.bat`，然后访问
<http://localhost:8787>。也可以直接运行 `artex.exe`。

请使用纯英文路径解压和保存数据，例如 `D:\Apps\ARTEX`（允许空格）。
随包 PostgreSQL 的 Windows 路径处理受系统代码页限制，中文安装/数据目录可能导致
初始化失败；数据库内容本身使用 UTF-8，可以正常保存中文。

不需要安装 Docker、PostgreSQL、Go 或 Node.js，也不需要管理员权限。
压缩包已经包含网页和免安装 PostgreSQL 17 运行组件，启动时不会下载数据库。
程序会自动创建数据库、选择本机空闲端口，并在退出时停止数据库。

## 数据保存

任务、设置和账号保存在 `data/postgres`，关闭或重启后仍然保留。
备份前退出 ARTEX，然后复制整个 `data` 目录。
`postgres` 是程序自带的运行组件目录，`data/postgres` 是你的数据，两者都应保留。
请勿删除密码文件或把数据放进临时目录。同一个 `data` 目录只能运行一个实例。

数据库仅监听 `127.0.0.1`，使用程序随机生成的密码，不注册系统服务。
主程序意外退出时，Windows 会清理它管理的数据库进程；再次启动时会恢复数据。

## 配置

压缩包中的 `config.json` 已设为 `database.driver = "embedded"`，无需填写连接串。
如需使用自行管理的 PostgreSQL，可设置 `ARTEX_PG_DSN`，该环境变量优先。
旧版本中的 `driver: "sqlite"` 配置不受支持，请使用新压缩包自带的配置。

不要只复制 `artex.exe`：须保留同目录下完整的 `postgres` 和 `skills` 文件夹。
更新时使用本仓库的 **Windows portable** 工作流产物；保留自己的 `data`，
不要覆盖成普通发行包中的可执行文件。

首次启动可能需要一些时间初始化，出错时查看 `data/postgres/server.log`。
默认网站端口 8787 被占用时可运行 `start.bat -addr :9000`。

## 流量捕获与代理

默认的 `127.0.0.1:8788` 是本机流量捕获监听，与设置页的「全局代理」不同。
「流量捕获」开启后，Agent 的目标流量通过这个监听录制；「全局代理」指定
外部出口，留空表示直连。关闭捕获且全局代理留空时，目标流量直接出网。
启动日志中的 `set HTTP_PROXY=...` 只是手动接入捕获的提示，不会修改 Windows 系统代理。

如需完全关闭 8788 监听，下次启动使用 `start.bat -proxy=`；重新启用需要重启程序。

## 第三方组件

PostgreSQL 由 Zonky 的 Windows 二进制发行包提供，打包时固定版本并校验 SHA256。
版权声明和来源在 `postgres/COPYRIGHT` 与 `postgres/SOURCE.txt`。
所需的 Microsoft Visual C++ 运行库随包放在组件目录，无需单独安装；
相关版权和许可说明见 `postgres/MICROSOFT_RUNTIME.txt`。

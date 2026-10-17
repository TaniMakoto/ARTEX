# Windows 免安装版

完整解压 `artex-windows-amd64-portable.zip`，双击 `start.bat`，然后访问
<http://localhost:8787>。也可以直接运行 `artex.exe`。

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

## 第三方组件

PostgreSQL 由 Zonky 的 Windows 二进制发行包提供，打包时固定版本并校验 SHA256。
版权声明和来源在 `postgres/COPYRIGHT` 与 `postgres/SOURCE.txt`。
所需的 Microsoft Visual C++ 运行库随包放在组件目录，无需单独安装；
相关版权和许可说明见 `postgres/MICROSOFT_RUNTIME.txt`。

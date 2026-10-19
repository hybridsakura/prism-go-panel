Prism Agent 便携启动版说明
==================================================

1. 特性：
   - 免安装，解压即用。
   - 不会向 Windows 系统注册系统服务，不占用系统服务列表。
   - 关闭命令行黑框窗口即停止运行，不会在后台驻留。

2. 配置文件与运行状态存储：
   默认自动存放于当前登录用户的应用数据目录：
   C:\Users\<当前用户>\AppData\Local\AkiraProgram\PrismGoPanel\state.json

3. 运行方式：
   方式一：直接双击运行 start-portable.bat，首次根据提示输入服务端地址和注册码即可。
   方式二：通过命令行执行：
   prism-agent.exe --server http://172.18.11.236:21114 --token <注册码>

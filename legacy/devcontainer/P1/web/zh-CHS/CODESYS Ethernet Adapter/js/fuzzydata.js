$(document).ready(function () {indexDict['zh-CHS'] = [{ "title" : "CODESYS Ethernet Adapter ", 
"url" : "_enad_start_page.html", 
"breadcrumbs" : "CODESYS Ethernet Adapter \/ CODESYS Ethernet Adapter ", 
"snippet" : "有关设备编辑器的以下选项卡的信息，请参阅一般说明。 “<设备名称> I\/O 映射”选项卡 “<设备名称> IEC 对象”选项卡 “<设备名称> 参数”选项卡 “<设备名称> 状态”选项卡 “<设备名称> 信息”选项卡 相关设备编辑器的附加单独帮助页面仅在特殊功能的情况下可用。 如果未显示“<设备名称>参数”选项卡，则选择 显示通用设备配置编辑器 中的选项 CODESYS 选项，在 设备编辑器 类别。 有关现场总线支持的更多一般信息，请参见 CODESYS ， 看： 现场总线支持 这 CODESYS 以太网适配器用作使用基于以太网协议的设备的父节点。 CODESYS 目前支持 EtherNet\/...", 
"body" : "有关设备编辑器的以下选项卡的信息，请参阅一般说明。 “<设备名称> I\/O 映射”选项卡 “<设备名称> IEC 对象”选项卡 “<设备名称> 参数”选项卡 “<设备名称> 状态”选项卡 “<设备名称> 信息”选项卡 相关设备编辑器的附加单独帮助页面仅在特殊功能的情况下可用。 如果未显示“<设备名称>参数”选项卡，则选择 显示通用设备配置编辑器 中的选项 CODESYS 选项，在 设备编辑器 类别。 有关现场总线支持的更多一般信息，请参见 CODESYS ， 看： 现场总线支持 这 CODESYS 以太网适配器用作使用基于以太网协议的设备的父节点。 CODESYS 目前支持 EtherNet\/IP、Modbus 和 PROFINET IO。 首先，将以太网适配器插入控制器下方。然后在适配器下方添加基于以太网的现场总线。 " }, 
{ "title" : "标签： EtherNet Adapter - 一般的 ", 
"url" : "_enad_edt_general.html", 
"breadcrumbs" : "CODESYS Ethernet Adapter \/ 标签： EtherNet Adapter - 一般的 ", 
"snippet" : "对象：以太网适配器 此选项卡定义网络接口。 网络接口 : 打开 网络适配器 对话。可以在此处选择以太网适配器，这些适配器在设置的目标系统上可用。 IP地址 子网掩码 默认网关 所选网络接口的设置 调整操作系统设置 您必须再次确认此选项。 注意：目标系统上的设置将被上述值覆盖。 操作系统的IP设置可以通过 CODESYS 运行时系统仅在配置文件允许的情况下（ *.cfg ) 的运行时。为此需要以下条目： [SysSocket] Adapter.0.Name=\"<adapter name>\" （例子： \"LAN connection 4\" ) Adapter.0.EnableSetIpAndMas...", 
"body" : "对象：以太网适配器 此选项卡定义网络接口。 网络接口 : 打开 网络适配器 对话。可以在此处选择以太网适配器，这些适配器在设置的目标系统上可用。 IP地址 子网掩码 默认网关 所选网络接口的设置 调整操作系统设置 您必须再次确认此选项。 注意：目标系统上的设置将被上述值覆盖。 操作系统的IP设置可以通过 CODESYS 运行时系统仅在配置文件允许的情况下（ *.cfg ) 的运行时。为此需要以下条目： [SysSocket] Adapter.0.Name=\"<adapter name>\" （例子： \"LAN connection 4\" ) Adapter.0.EnableSetIpAndMask=1 冗余 PLC 设置（PLC 2） 使用一对冗余 CODESYS 控制器时，这些设置用于“备用 PLC”（ PLC ID = 2 ) 网络适配器。 网络接口 ：打开 网络适配器 对话。可以在此处选择以太网适配器，这些适配器在设置的目标系统上可用。 IP地址 子网掩码 默认网关 所选网络接口的设置 冗余 PLC 的设置仅在您插入 Redundancy Configuration 应用程序下方的对象。 " }, 
{ "title" : "对话框：网络适配器 ", 
"url" : "_enad_edt_general.html#UUID-68dc7536-40d9-f126-1da8-b5aefc635b70_id_eb69772dd959aeac0a8640e01875adb_id_98733dd8635a8e3dc0a8640e011b6259", 
"breadcrumbs" : "CODESYS Ethernet Adapter \/ 标签： EtherNet Adapter - 一般的 \/ 对话框：网络适配器 ", 
"snippet" : "接口 显示所有可用网络适配器的概览。 IP地址 子网掩码 默认网关 这些参数取自操作系统，不能修改。 MAC地址 以太网适配器的 MAC 地址...", 
"body" : "接口 显示所有可用网络适配器的概览。 IP地址 子网掩码 默认网关 这些参数取自操作系统，不能修改。 MAC地址 以太网适配器的 MAC 地址 " }
]
$(document).trigger('search.ready');
});
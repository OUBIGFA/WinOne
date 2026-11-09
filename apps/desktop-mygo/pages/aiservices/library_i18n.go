package aiservices

import (
	"strings"
	"winone/desktopmygo/i18n"
)

func externalT(key string) string {
	words := map[string][2]string{
		"read": {"读取客户端资料", "Read client data"}, "disconnect": {"停止读取", "Disconnect"},
		"boundary":      {"点击“读取客户端资料”后才读取本机客户端目录。保存、安装、同步、转换与导出仅在点击对应操作后执行。", "Client folders are read only after you click Read client data. Save, install, sync, convert and export run only when requested."},
		"writeBoundary": {"此操作按所选项目更改客户端文件或安装内容。上游保留备份；请核对目标后确认。", "This action changes the selected client files or installs content. Upstream backups are retained. Check the targets before confirming."},
		"saved":         {"操作已完成。", "Action completed."}, "agents": {"分配给客户端", "Give to clients"}, "all": {"全选", "Select all"}, "none": {"全不选", "Select none"},
		"lines": {"每行一项", "One item per line"}, "pairs": {"每行 名称=值", "One name=value per line"}, "actions": {"更多操作", "More actions"},
		"stats": {"读取统计", "Read statistics"}, "transcript": {"读取对话", "Read conversation"}, "access": {"会话读取范围", "Session access"}, "allow": {"允许读取选定客户端", "Allow selected clients"},
		"allClients": {"全部客户端", "All clients"}, "pause": {"暂停读取全部会话", "Pause session reads"}, "found": {"客户端已有内容", "Found in clients"}, "selected": {"选中项目", "Selected items"},
		"preview": {"预览", "Preview"}, "files": {"文件", "Files"}, "copyCommand": {"复制继续命令", "Copy resume command"}, "next": {"下一页", "Next page"}, "previous": {"上一页", "Previous page"},
		"name": {"名称", "Name"}, "empty": {"没有匹配项目。", "No matching items."}, "add": {"添加", "Add"}, "readRTK": {"检查 RTK", "Check RTK"}, "market": {"搜索市场", "Search market"},
		"startExport": {"开始导出会话轨迹", "Start exporting agent conversations"}, "stopExport": {"停止导出会话轨迹", "Stop exporting agent conversations"}, "exportHint": {"使用已保存的 OpenTelemetry 接收地址与正文选项。只有允许读取的客户端会话会被发送。", "Uses the saved OpenTelemetry destination and content options. Only conversations from allowed clients are sent."},
	}
	if pair, ok := words[key]; ok {
		if strings.HasPrefix(i18n.Locale(), "zh") {
			return pair[0]
		}
		return pair[1]
	}
	return upstreamT(key)
}

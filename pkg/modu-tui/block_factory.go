package modutui

func defaultBlockFromEntry(entry Entry) Block {
	return nodeGroupBlock{
		Marker: entryMarker(entry),
		Nodes:  entry.Nodes,
		Dim:    entry.Role == RoleStatus,
	}
}

func entryMarker(entry Entry) string {
	if entry.Plain {
		return ""
	}
	switch entry.Role {
	case RoleUser:
		return youStyle.Render("❯ ")
	case RoleStatus:
		return statusStyle.Render("· ")
	}
	return assistantMarkerStyle.Render("● ")
}

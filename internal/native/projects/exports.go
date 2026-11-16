package projects

import (
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/project"
)

// exportDialog is *Export…* of a project (SPEC 2.1, export-project.tsx):
// a zip file or a folder, with or without its approvals, the size first,
// then the progress, then how it ended. It stays open while the export
// runs; Stop stops it.
func (s *Screens) exportDialog(c *kit.Context) {
	st := s.Stores
	p := s.exportFor
	if p == "" {
		return
	}
	x := st.Exports.Projects[p]
	if !s.exportOpen || x == nil {
		st.Exports.Close(p)
		s.exportFor, s.exportOpen = "", false
		return
	}
	name := s.ProjectName(p)
	kit.Dialog(c, kit.Locked(&s.exportOpen, x.Running), "Export project", func() {
		kit.DialogText(c, "A copy of "+name+" as it is now: its data, chats and files. You can keep working while it runs.")
		k := kit.TokensOf(c)
		switch {
		case x.Result != nil:
			r := x.Result
			kit.Column(c).Gap(8).AlignItems(kit.Stretch).FontSize(14).Children(func() {
				kit.Text(c, "Exported "+size(r.Size)+" in "+seconds(r.Millis)+".")
				kit.Path(c, r.Path).TextColor(k.MutedForeground)
				if r.Missing > 0 {
					kit.Alert(c, kit.Amber, MissingText(r.Missing), "")
				}
				kit.Text(c, "Copied into the projects folder, the export opens as a new project next to this one.").TextColor(k.MutedForeground)
			})
		case x.Running:
			kit.Column(c).Gap(8).AlignItems(kit.Stretch).FontSize(14).Children(func() {
				kit.Row(c).Gap(8).Justify(kit.SpaceBetween).AlignItems(kit.Center).Children(func() {
					step := StepText("")
					if x.Progress != nil {
						step = StepText(x.Progress.Step)
					}
					if x.Stopping {
						step = "Stopping…"
					}
					kit.Text(c, step)
					if x.Progress != nil {
						kit.Text(c, size(x.Progress.Done)+" / "+size(x.Progress.Total)).TextColor(k.MutedForeground)
					}
				})
				kit.Progress(c, Percent(x.Progress)/100, "Export progress")
			})
		default:
			kit.Column(c).Gap(12).AlignItems(kit.Stretch).FontSize(14).Children(func() {
				kit.SegmentGroup(c, &s.exportFormat, "Export as", []kit.Segment{
					{Value: string(project.ExportZip), Label: "Zip file", Icon: kit.IconFileArchive},
					{Value: string(project.ExportFolder), Label: "Folder", Icon: kit.IconProjectFolder},
				}).AlignSelf(kit.Start)
				kit.CheckField(c, &s.approvals, "Include approvals (for my own backup)",
					"The hosts, scripts and tools you allowed, the approval level and the private-network exceptions. Without them, the copy asks again like a new project.")
				if e := x.Estimate; e != nil {
					kit.Text(c, "About "+size(e.Bytes)+" before compression, with "+plural(int64(e.Objects), "file", "files")+".").TextColor(k.MutedForeground)
				} else {
					kit.SmallSpinner(c, "Adding up the size")
				}
			})
		}
		if x.Error != "" {
			kit.Alert(c, kit.Amber, x.Error, "")
		}
		kit.DialogFooter(c, func() {
			switch {
			case x.Result != nil:
				if kit.Button(c, "Done", kit.Default).Clicked() {
					s.exportOpen = false
				}
			case x.Running:
				if kit.Disable(kit.ActionButton(c, nil, "Stop", kit.Outline, x.Stopping), x.Stopping).Clicked() {
					st.Exports.Stop(p)
				}
			default:
				if kit.Button(c, "Cancel", kit.Outline).Clicked() {
					s.exportOpen = false
				}
				if kit.Disable(kit.Button(c, "Export…", kit.Default), x.Estimate == nil).Clicked() && x.Estimate != nil {
					st.Exports.Start(p, name, project.ExportFormat(s.exportFormat), s.approvals)
				}
			}
		})
	})
}

// chatExportDialog is *Export chat…* (SPEC 2.3, export-chat.tsx): one chat
// as a Markdown file to read, or a JSON file that keeps every part.
// Secrets are replaced in both.
func (s *Screens) chatExportDialog(c *kit.Context) {
	st := s.Stores
	t := s.chatFor
	if t.chat == "" {
		return
	}
	x := st.Exports.Chats[t.chat]
	if !s.chatOpen || x == nil {
		s.chatFor, s.chatOpen = chatTarget{}, false
		return
	}
	title := t.title
	if title == "" {
		title = "New chat"
	}
	kit.Dialog(c, kit.Locked(&s.chatOpen, x.Running), "Export chat", func() {
		kit.DialogText(c, "Save "+title+" as a file. Keys are replaced with their names.")
		k := kit.TokensOf(c)
		if r := x.Result; r != nil {
			kit.Column(c).Gap(8).AlignItems(kit.Stretch).FontSize(14).Children(func() {
				kit.Text(c, "Exported "+plural(int64(r.Messages), "message", "messages")+", "+size(r.Size)+".")
				kit.Path(c, r.Path).TextColor(k.MutedForeground)
			})
		} else {
			kit.Column(c).Gap(12).AlignItems(kit.Stretch).FontSize(14).Children(func() {
				kit.Disable(kit.SegmentGroup(c, &s.chatFormat, "Export as", []kit.Segment{
					{Value: string(chatexport.FormatMarkdown), Label: "Markdown", Icon: kit.IconFileText},
					{Value: string(chatexport.FormatJSON), Label: "JSON", Icon: kit.IconFileJSON},
				}), x.Running).AlignSelf(kit.Start)
				kit.Text(c, FormatText(chatexport.Format(s.chatFormat))).TextColor(k.MutedForeground)
			})
		}
		if x.Error != "" {
			kit.Alert(c, kit.Amber, x.Error, "")
		}
		kit.DialogFooter(c, func() {
			if x.Result != nil {
				if kit.Button(c, "Done", kit.Default).Clicked() {
					s.chatOpen = false
				}
				return
			}
			if kit.Disable(kit.Button(c, "Cancel", kit.Outline), x.Running).Clicked() {
				s.chatOpen = false
			}
			if kit.ActionButton(c, nil, "Export…", kit.Default, x.Running).Clicked() {
				st.Exports.StartChat(t.project, t.chat, t.title, chatexport.Format(s.chatFormat))
			}
		})
	})
}

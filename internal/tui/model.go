package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
)

type state int

const (
	stateMain          state = iota // dual pane: projects (left) + shared docs (right)
	stateProjectDetail             // dual pane: project files (left) + shared refs (right)
	stateSharedFiles               // single pane: files in a shared doc
)

type pane int

const (
	paneLeft pane = iota
	paneRight
)

type model struct {
	state  state
	focus  pane
	kbRoot string

	projectName string
	editor      string

	// stateMain
	projectsList list.Model // left
	sharedList   list.Model // right

	// stateProjectDetail
	detailList    list.Model // left (project files)
	sharedRefList list.Model // right (all shared docs with ref status)

	// stateSharedFiles
	sharedFilesList list.Model // single pane

	width, height int
	err           error
}

// New creates a new TUI model. If projectName is non-empty, starts on project detail view.
func New(kbRoot, projectName string) model {
	editor := "vi"
	if cfg, err := config.Load(kbRoot); err == nil {
		editor = cfg.GetEditor()
	}

	m := model{
		kbRoot:      kbRoot,
		projectName: projectName,
		editor:      editor,
	}

	delegate := list.NewDefaultDelegate()

	m.projectsList = list.New(nil, delegate, 0, 0)
	m.projectsList.Title = "Projects"
	m.projectsList.SetShowHelp(false)
	m.projectsList.SetFilteringEnabled(true)

	m.sharedList = list.New(nil, delegate, 0, 0)
	m.sharedList.Title = "Shared Docs"
	m.sharedList.SetShowHelp(false)
	m.sharedList.SetFilteringEnabled(true)

	m.detailList = list.New(nil, delegate, 0, 0)
	m.detailList.SetShowHelp(false)
	m.detailList.SetFilteringEnabled(true)

	m.sharedRefList = list.New(nil, delegate, 0, 0)
	m.sharedRefList.Title = "Shared Refs"
	m.sharedRefList.SetShowHelp(false)
	m.sharedRefList.SetFilteringEnabled(true)

	m.sharedFilesList = list.New(nil, delegate, 0, 0)
	m.sharedFilesList.SetShowHelp(false)
	m.sharedFilesList.SetFilteringEnabled(true)

	if projectName != "" {
		m.state = stateProjectDetail
	} else {
		m.state = stateMain
	}

	return m
}

// --- Messages ---

type loadProjectsMsg struct {
	items []list.Item
	err   error
}

type loadSharedMsg struct {
	items []list.Item
	err   error
}

type loadProjectDetailMsg struct {
	projectName string
	fileItems   []list.Item // project files for left pane
	refItems    []list.Item // all shared docs with linked status for right pane
	err         error
}

type loadSharedFilesMsg struct {
	title string
	items []list.Item
	err   error
}

type refToggledMsg struct{ err error }

type editorFinishedMsg struct{ err error }

// --- Init ---

func (m model) Init() tea.Cmd {
	load := tea.Batch(m.loadProjects(), m.loadShared())
	if m.state == stateProjectDetail {
		return tea.Batch(load, m.loadProjectDetail(m.projectName))
	}
	return load
}

// --- Commands ---

func (m model) loadProjects() tea.Cmd {
	return func() tea.Msg {
		projects, err := project.List(m.kbRoot)
		if err != nil {
			return loadProjectsMsg{err: err}
		}
		items := make([]list.Item, len(projects))
		for i, p := range projects {
			items[i] = projectItem{info: p}
		}
		return loadProjectsMsg{items: items}
	}
}

func (m model) loadShared() tea.Cmd {
	return func() tea.Msg {
		docs, err := shared.List(m.kbRoot)
		if err != nil {
			return loadSharedMsg{err: err}
		}
		items := make([]list.Item, len(docs))
		for i, d := range docs {
			items[i] = sharedItem{info: d}
		}
		return loadSharedMsg{items: items}
	}
}

func (m model) loadProjectDetail(name string) tea.Cmd {
	return func() tea.Msg {
		info, err := project.Get(m.kbRoot, name)
		if err != nil {
			return loadProjectDetailMsg{err: err}
		}

		// Left pane: project files
		var fileItems []list.Item
		projDir := project.Dir(m.kbRoot, name)

		contextPath := filepath.Join(projDir, "context.md")
		if fs.FileExists(contextPath) {
			fileItems = append(fileItems, fileItem{
				name: "context.md",
				path: contextPath,
			})
		}
		for _, f := range info.Files {
			fileItems = append(fileItems, fileItem{
				name: f,
				path: filepath.Join(projDir, f),
			})
		}

		// Right pane: all shared docs with linked status
		docs, _ := shared.List(m.kbRoot)
		refSet := make(map[string]bool)
		for _, r := range info.Refs {
			refSet[r] = true
		}
		var refItems []list.Item
		for _, d := range docs {
			refItems = append(refItems, sharedRefItem{
				info:   d,
				linked: refSet[d.Slug],
			})
		}

		return loadProjectDetailMsg{
			projectName: name,
			fileItems:   fileItems,
			refItems:    refItems,
		}
	}
}

func (m model) loadSharedFiles(slug string) tea.Cmd {
	return func() tea.Msg {
		info, err := shared.Get(m.kbRoot, slug)
		if err != nil {
			return loadSharedFilesMsg{err: err}
		}
		sharedDir := shared.Dir(m.kbRoot, slug)
		var items []list.Item
		for _, f := range info.Files {
			items = append(items, fileItem{
				name:     f,
				path:     filepath.Join(sharedDir, f),
				isShared: true,
				slug:     slug,
			})
		}
		return loadSharedFilesMsg{title: info.DisplayTitle(), items: items}
	}
}

func (m model) toggleRef(slug string, linked bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		action := "link"
		if linked {
			action = "unlink"
			err = project.RemoveRef(m.kbRoot, m.projectName, slug)
		} else {
			err = project.AddRef(m.kbRoot, m.projectName, slug)
		}
		if err != nil {
			return refToggledMsg{err: err}
		}
		_ = git.AutoCommit(m.kbRoot, fmt.Sprintf("ref: %s %s → %s", action, m.projectName, slug))
		return refToggledMsg{}
	}
}

func (m model) openEditor(path string) tea.Cmd {
	c := exec.Command(m.editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// --- Update ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		halfW := m.width / 2
		listH := m.height - 2
		// Dual-pane lists: subtract 1 for the left border
		m.projectsList.SetSize(halfW-1, listH)
		m.sharedList.SetSize(m.width-halfW-1, listH)
		m.detailList.SetSize(halfW-1, listH)
		m.sharedRefList.SetSize(m.width-halfW-1, listH)
		m.sharedFilesList.SetSize(m.width, listH)
		return m, nil

	case loadProjectsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.projectsList.SetItems(msg.items)
		return m, nil

	case loadSharedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.sharedList.SetItems(msg.items)
		return m, nil

	case loadProjectDetailMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.projectName = msg.projectName
		m.detailList.Title = msg.projectName
		m.detailList.SetItems(msg.fileItems)
		m.sharedRefList.SetItems(msg.refItems)
		return m, nil

	case loadSharedFilesMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.sharedFilesList.Title = msg.title
		m.sharedFilesList.SetItems(msg.items)
		return m, nil

	case refToggledMsg:
		if msg.err != nil {
			cmd := m.sharedRefList.NewStatusMessage(fmt.Sprintf("Error: %v", msg.err))
			return m, cmd
		}
		return m, m.loadProjectDetail(m.projectName)

	case editorFinishedMsg:
		if msg.err != nil {
			var cmd tea.Cmd
			switch m.state {
			case stateProjectDetail:
				cmd = m.detailList.NewStatusMessage(fmt.Sprintf("Editor error: %v", msg.err))
			case stateSharedFiles:
				cmd = m.sharedFilesList.NewStatusMessage(fmt.Sprintf("Editor error: %v", msg.err))
			}
			return m, cmd
		}
		return m, nil
	}

	switch m.state {
	case stateMain:
		return m.updateMain(msg)
	case stateProjectDetail:
		return m.updateProjectDetail(msg)
	case stateSharedFiles:
		return m.updateSharedFiles(msg)
	}
	return m, nil
}

func (m model) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		// Don't intercept keys while filtering.
		if m.focus == paneLeft && m.projectsList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.projectsList, cmd = m.projectsList.Update(msg)
			return m, cmd
		}
		if m.focus == paneRight && m.sharedList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.sharedList, cmd = m.sharedList.Update(msg)
			return m, cmd
		}

		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left", "h":
			m.focus = paneLeft
			return m, nil
		case "right", "l":
			m.focus = paneRight
			return m, nil
		case "tab":
			if m.focus == paneLeft {
				m.focus = paneRight
			} else {
				m.focus = paneLeft
			}
			return m, nil
		case "esc":
			if m.focus == paneLeft && m.projectsList.FilterState() == list.FilterApplied {
				m.projectsList.ResetFilter()
				return m, nil
			}
			if m.focus == paneRight && m.sharedList.FilterState() == list.FilterApplied {
				m.sharedList.ResetFilter()
				return m, nil
			}
			return m, nil
		case "enter":
			if m.focus == paneLeft {
				item, ok := m.projectsList.SelectedItem().(projectItem)
				if ok {
					m.state = stateProjectDetail
					m.focus = paneLeft
					return m, m.loadProjectDetail(item.info.Name)
				}
			} else {
				item, ok := m.sharedList.SelectedItem().(sharedItem)
				if ok {
					m.state = stateSharedFiles
					return m, m.loadSharedFiles(item.info.Slug)
				}
			}
		}
	}

	// Forward to focused list.
	var cmd tea.Cmd
	if m.focus == paneLeft {
		m.projectsList, cmd = m.projectsList.Update(msg)
	} else {
		m.sharedList, cmd = m.sharedList.Update(msg)
	}
	return m, cmd
}

func (m model) updateProjectDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.focus == paneLeft && m.detailList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.detailList, cmd = m.detailList.Update(msg)
			return m, cmd
		}
		if m.focus == paneRight && m.sharedRefList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.sharedRefList, cmd = m.sharedRefList.Update(msg)
			return m, cmd
		}

		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left", "h":
			m.focus = paneLeft
			return m, nil
		case "right", "l":
			m.focus = paneRight
			return m, nil
		case "tab":
			if m.focus == paneLeft {
				m.focus = paneRight
			} else {
				m.focus = paneLeft
			}
			return m, nil
		case "esc":
			if m.focus == paneLeft && m.detailList.FilterState() == list.FilterApplied {
				m.detailList.ResetFilter()
				return m, nil
			}
			if m.focus == paneRight && m.sharedRefList.FilterState() == list.FilterApplied {
				m.sharedRefList.ResetFilter()
				return m, nil
			}
			m.state = stateMain
			m.focus = paneLeft
			return m, nil
		case "enter":
			if m.focus == paneLeft {
				item, ok := m.detailList.SelectedItem().(fileItem)
				if ok {
					return m, m.openEditor(item.path)
				}
			} else {
				item, ok := m.sharedRefList.SelectedItem().(sharedRefItem)
				if ok {
					return m, m.toggleRef(item.info.Slug, item.linked)
				}
			}
		}
	}

	var cmd tea.Cmd
	if m.focus == paneLeft {
		m.detailList, cmd = m.detailList.Update(msg)
	} else {
		m.sharedRefList, cmd = m.sharedRefList.Update(msg)
	}
	return m, cmd
}

func (m model) updateSharedFiles(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.sharedFilesList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.sharedFilesList, cmd = m.sharedFilesList.Update(msg)
			return m, cmd
		}
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.sharedFilesList.FilterState() == list.FilterApplied {
				m.sharedFilesList.ResetFilter()
				return m, nil
			}
			m.state = stateMain
			m.focus = paneRight
			return m, nil
		case "enter":
			item, ok := m.sharedFilesList.SelectedItem().(fileItem)
			if ok {
				return m, m.openEditor(item.path)
			}
		}
	}

	var cmd tea.Cmd
	m.sharedFilesList, cmd = m.sharedFilesList.Update(msg)
	return m, cmd
}

// --- View ---

// ErrorReporter allows callers to check if the model exited with an error.
type ErrorReporter interface {
	Err() error
}

// Err returns any error that caused the TUI to exit.
func (m model) Err() error { return m.err }

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	switch m.state {
	case stateMain:
		return m.viewDualPane(
			m.projectsList.View(), m.sharedList.View(),
			"enter: open  ←→/hl: switch pane  /: filter  q: quit",
		)
	case stateProjectDetail:
		return m.viewDualPane(
			m.detailList.View(), m.sharedRefList.View(),
			"enter: edit/toggle ref  ←→/hl: switch pane  /: filter  esc: back  q: quit",
		)
	case stateSharedFiles:
		return m.sharedFilesList.View() + "\n" +
			helpStyle.Render("enter: edit  esc: back  /: filter  q: quit")
	}
	return ""
}

func (m model) viewDualPane(left, right, help string) string {
	halfW := m.width / 2
	listH := m.height - 2

	leftStyle := unfocusedPane.Width(halfW - 1).Height(listH)
	rightStyle := unfocusedPane.Width(m.width - halfW - 1).Height(listH)
	if m.focus == paneLeft {
		leftStyle = focusedPane.Width(halfW - 1).Height(listH)
	} else {
		rightStyle = focusedPane.Width(m.width - halfW - 1).Height(listH)
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		leftStyle.Render(left),
		rightStyle.Render(right),
	)
	return panes + "\n" + helpStyle.Render(help)
}

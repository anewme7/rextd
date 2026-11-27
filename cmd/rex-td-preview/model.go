package main

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"rex-td/rex"
)

type taskState int

const (
	todo taskState = iota
	working
	paused
	done
	cancelled
)

var stateLabels = [...]string{"待做", "进行中", "暂缓", "已完成", "已取消"}
var densityLabels = [...]string{"紧凑", "标准", "完整"}
var colorLabels = [...]string{"默认", "蓝", "绿", "琥珀", "紫"}

type taskKind int

const (
	uncategorized taskKind = iota
	feature
	repair
	docsKind
	operations
	releaseKind
)

var kindLabels = [...]string{"未分类", "功能", "修复", "文档", "运维", "发布"}

type task struct {
	id                                   int64
	title, note                          string
	state, previousState                 taskState
	archived                             bool
	sortOrder, parentID, completionOrder int64
	pinned                               bool
	example                              bool
	color                                int
	kind                                 taskKind
	steps                                []string
	completed                            int
	panes                                []rex.Pane
	bindings                             map[paneIdentity]paneBinding
	result                               string
	resultCount                          int
}

type page int

const (
	listPage page = iota
	detailPage
	scenePage
	helpPage
)

type homeClickSpan struct {
	start, end int
	rowOffset  int
	cmd        string
}

type homeDrawerHit struct {
	row, start, end int
	focus, hostSlot int
}

type model struct {
	genericDirs             map[string]bool
	autolinkDirty           bool
	boardSuggestionPane     paneIdentity
	autolinkStarted         time.Time
	observedAgents          map[paneIdentity]agentObservation
	taskTouched             map[int64]int64
	newAgents               []rex.Pane
	suggestedTask           int64
	readingDirectories      bool
	openingTaskTab          int64
	removedAgents           map[paneIdentity]bool
	boardSuggest            bool
	boardOfferOpen          bool
	boardOfferSelected      bool
	homeTab                 taskTab
	lastActiveTab           taskTab
	homeOverview            bool
	homeScrollTop           int
	homeScrollMax           int
	homeBodyEnd             int
	homeScrollManual        bool
	lastWheel               tea.MouseButton
	lastWheelAt             time.Time
	overviewFolded          [4]bool
	overviewSectionSelected bool
	overviewSectionTab      taskTab
	secondaryOpen           [4]bool
	secondarySelected       bool
	boardPane               paneIdentity
	boardMore               bool
	boardOpenIdle           map[*task]bool
	homeGraphics            *homeGraphics
	homePlacements          []homePlacement
	paletteStoreVersion     int64
	paletteResume           paletteResume
	paletteResumeApplied    int64
	archivedView            bool
	menuBack                page
	menuSelection           int
	taskDelete              taskDeletion
	taskEdit                *taskEdit
	taskPick                *taskPick
	focusPreview            bool
	terminalPreview         *terminalPreview
	previews                *rex.TerminalPreviews
	pickerAll               bool
	pickerSelection         int
	replacePane             rex.Pane
	demo                    bool
	language                language
	tasks                   []*task
	store                   *taskStore
	expanded                map[*task]bool
	taskSelection           int
	detailSelection         int
	detailIdlePane          paneIdentity
	paneSelection           int
	grouping                grouping
	allPanes                bool
	panePins                map[paneIdentity]bool
	seenResults             map[paneIdentity]seenResult
	foldedTasks             map[string]bool
	foldedPanes             map[string]bool
	density                 int
	focus                   int
	hostOffset              int
	quotaOffset             int
	attention               bool
	peek                    bool
	helpOffset              int
	page                    page
	sceneBack               page
	editing                 bool
	input                   lineInput
	scene                   *paneScene
	width, height           int
	updates                 <-chan rex.Fleet
	feed                    rex.Fleet
	livePanes               []rex.Pane
	hostFilter              string
	startupHost             int
	hostSelection           int
	notice                  string
	jumping                 bool
	ctx                     context.Context
	rexContext              rex.Context
	metrics                 *metricFeed
	metricWorkers           sync.WaitGroup
	codexRefreshAfter       time.Time
	hostCPU                 map[string]*int
	claudeUsage, codexUsage quotaReading
	claudeLimit             quotaReading
	codexFailed             bool
	mouseRows               []listHit
	mouseBodyStart          int
	mouseResourceTitle      int
	homeHeaderHits          []homeClickSpan
	homeTopHits             []homeClickSpan
	homeFooterHits          []homeClickSpan
	homeDrawerHits          []homeDrawerHit
	candidates              []taskCandidate
	candidateRequests       map[paneIdentity]string
	candidateIgnored        map[paneIdentity]string
	candidateWatches        map[paneIdentity]bool
	candidatesOpen          bool
	candidatesFocused       bool
	candidatePane           paneIdentity
	candidateDraft          *taskCandidate
}

func newModel(demo bool) *model {
	m := &model{demo: demo, language: chinese, expanded: make(map[*task]bool), panePins: make(map[paneIdentity]bool),
		foldedTasks: make(map[string]bool), foldedPanes: make(map[string]bool),
		seenResults: make(map[paneIdentity]seenResult),
		hostCPU:     make(map[string]*int), density: 1,
		candidateRequests: make(map[paneIdentity]string), candidateIgnored: make(map[paneIdentity]string),
		candidateWatches: make(map[paneIdentity]bool),
		autolinkStarted:  time.Now(), observedAgents: make(map[paneIdentity]agentObservation),
		taskTouched: make(map[int64]int64), removedAgents: make(map[paneIdentity]bool),
		overviewFolded: [4]bool{doneTab: true, archiveTab: true}}
	if demo {
		m.feed, m.tasks = sampleData()
		for i, task := range m.tasks {
			task.example, task.id = true, int64(i+1)
		}
		m.refreshPanes()
	}
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.watch(), m.watchMetrics(), m.watchPaletteStore())
}

func (m *model) watchMetrics() tea.Cmd {
	f := m.metrics
	if f == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case event := <-f.messages:
			return metricEvent{feed: f, value: event}
		case <-f.ctx.Done():
			return nil
		}
	}
}

func (m *model) watch() tea.Cmd {
	if m.updates == nil {
		return nil
	}
	return func() tea.Msg {
		snapshot, ok := <-m.updates
		if !ok {
			return nil
		}
		return snapshot
	}
}

type jumpResult struct{ err error }
type openHome struct{}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if m.autolinkDirty {
		cmd = tea.Batch(cmd, m.syncAutolink())
	}
	return next, tea.Batch(cmd, m.syncMetrics())
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if event, ok := msg.(metricEvent); ok {
		if event.feed != m.metrics {
			return m, nil
		}
		msg = event.value
	}
	switch msg := msg.(type) {
	case terminalThemeMsg:
		lipgloss.SetHasDarkBackground(msg.dark)
		return m, nil
	case terminalPreviewUpdate:
		return m, m.receiveTerminalPreview(msg)
	case paletteStoreTick:
		changed := m.refreshPaletteStore()
		m.applyPaletteResume()
		if changed {
			return m, tea.Batch(m.watchPaletteStore(), m.syncAutolink())
		}
		return m, m.watchPaletteStore()
	case openHome:
		m.startupHost = 0
		if !m.editing && (m.scene == nil || !m.scene.editing) && m.page != taskPickPage && m.page != taskDeletePage {
			// A Find request may already be committed but not yet polled.
			// Home supersedes that request, including while leaving a preview.
			if m.store != nil {
				m.refreshPaletteStore()
				m.paletteResumeApplied = m.paletteResume.revision
			}
			m.closeTerminalPreview()
			m.cancelPaneTurn()
			m.page, m.attention, m.archivedView = listPage, false, false
			m.focus, m.peek, m.notice = 0, false, ""
			m.candidatesFocused = false
			m.boardSuggest, m.boardMore, m.boardPane, m.boardSuggestionPane = false, false, paneIdentity{}, paneIdentity{}
			m.boardOfferOpen, m.boardOfferSelected = false, false
			m.switchHomeTab(nowTab)
		}
	case taskTabResult:
		m.openingTaskTab = 0
		m.finishTaskTab(msg)
		return m, m.syncAutolink()
	case paneDirectoryResult:
		m.readingDirectories = false
		for id, reply := range msg {
			observation, seen := m.observedAgents[id]
			if seen && observation.app == reply.app {
				observation.dir, observation.directoryReadAt = reply.dir, reply.directoryReadAt
				m.observedAgents[id] = observation
			}
		}
		return m, m.syncAutolink()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if p := m.terminalPreview; p != nil && p.lines != nil {
			p.layout(msg.Width)
		}
		m.mouseRows = nil
		m.mouseResourceTitle = -1
	case tea.MouseMsg:
		m.startupHost = 0
		if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			// A touch fling may send dozens of wheel reports in a single frame.
			now := time.Now()
			if m.lastWheel == msg.Button && now.Sub(m.lastWheelAt) < time.Second/24 {
				return m, nil
			}
			m.lastWheel, m.lastWheelAt = msg.Button, now
		}
		if m.terminalPreview != nil {
			return m, m.terminalPreviewMouse(msg)
		}
		return m, m.handleMouse(msg)
	case rex.Fleet:
		var target paneIdentity
		if m.page == linkPage {
			panes := m.pickable()
			if i := m.pickerSelection; i >= 0 && i < len(panes) {
				target = identityOf(panes[i])
			}
		}
		m.feed = msg
		if m.startupHost > 0 && m.startupHost <= len(msg.Hosts) {
			m.showHost(m.startupHost)
			m.startupHost = 0
		}
		m.pruneSeenResults()
		if target != (paneIdentity{}) {
			m.pickerSelection = slices.IndexFunc(m.pickable(), func(p rex.Pane) bool { return identityOf(p) == target })
			if m.pickerSelection < 0 {
				m.notice = m.text("窗格已变化，请重新选择")
			}
		}
		m.claudeLimit = claudeWeeklyLimit(msg.Panes)
		m.refreshPanes()
		m.refreshCandidates()
		if m.metrics != nil {
			m.metrics.addHosts(msg.Hosts)
		}
		return m, tea.Batch(m.watch(), m.syncAutolink())
	case cpuUpdate:
		m.hostCPU[msg.host] = msg.value
		return m, m.watchMetrics()
	case claudeUpdate:
		m.claudeUsage = msg.reading
		return m, m.watchMetrics()
	case codexUpdate:
		m.codexRefreshAfter = msg.refreshAfter
		m.codexFailed = msg.failed
		if !msg.failed {
			m.codexUsage = msg.reading
		}
		return m, m.watchMetrics()
	case jumpResult:
		m.jumping = false
		if msg.err != nil {
			m.notice = m.text("跳转失败 · ") + m.text(msg.err.Error())
		} else {
			m.notice = m.text("已定位到 Rex 窗格")
		}
	case paneAgentResult:
		m.receivePaneAgent(msg)
	case tea.KeyMsg:
		m.startupHost = 0
		m.homeScrollManual = false
		if msg.Type == tea.KeyCtrlC {
			m.closeTerminalPreview()
			return m, tea.Quit
		}
		if m.terminalPreview != nil {
			return m, m.terminalPreviewKey(msg.String())
		}
		if m.page == scenePage && m.scene.editing {
			return m, m.editPaneAgent(msg)
		}
		if m.page == taskPickPage {
			m.taskPickKey(msg)
			return m, nil
		}
		if m.page == taskDeletePage {
			m.taskDeleteKey(msg.String())
			return m, nil
		}
		if msg.Type == tea.KeyRunes && msg.Alt && len(msg.Runes) == 1 &&
			msg.Runes[0] >= '0' && msg.Runes[0] <= '9' {
			if !m.editing {
				m.showHost(int(msg.Runes[0] - '0'))
			}
			return m, nil
		}
		if m.editing {
			m.edit(msg)
			return m, nil
		}
		key := msg.String()
		if key == "L" {
			if m.language == english {
				m.language = chinese
			} else {
				m.language = english
			}
			m.notice = ""
			return m, nil
		}
		if key == "q" {
			return m, tea.Quit
		}
		return m, m.listCommand(key)
	}
	return m, nil
}

func (m *model) listCommand(key string) tea.Cmd {
	if m.page == taskDeletePage {
		m.taskDeleteKey(key)
		return nil
	}
	if m.page == scenePage {
		switch {
		case key == "i" && m.scene.pane.App == "codex" && !m.demo:
			m.cancelPaneTurn()
			m.scene.request++
			m.scene.editing = true
			m.scene.input.set(m.scene.binding.id)
			m.notice = ""
			return nil
		case key == "y":
			m.toggleSeenResult()
			return nil
		case key == "m":
			m.scene.more, m.scene.offset = !m.scene.more, 0
			return nil
		case key == "u":
			return m.readPaneAgent("", false)
		}
	}
	if m.focusPreview && m.page == listPage && m.focus == 0 {
		switch key {
		case "1", "2", "3", "4":
			tab := taskTab(key[0] - '1')
			if m.homeOverview && !m.attention {
				m.jumpOverviewSection(tab)
			} else {
				m.switchHomeTab(tab)
			}
			return nil
		case "tab", "shift+tab":
			if m.homeOverview && !m.attention {
				step := 1
				if key == "shift+tab" {
					step = len(homeTabNames) - 1
				}
				cur := m.currentOverviewTab()
				m.jumpOverviewSection((cur + taskTab(step)) % taskTab(len(homeTabNames)))
				return nil
			}
			step := 1
			if key == "shift+tab" {
				step = len(homeTabNames) - 1
			}
			m.switchHomeTab((m.homeTab + taskTab(step)) % taskTab(len(homeTabNames)))
			return nil
		case "esc":
			if m.attention {
				m.switchHomeTab(m.homeTab)
				return nil
			}
		case "n":
			m.hostFilter = ""
			m.grouping = byStatus
		}
	}
	if m.onCandidates() && m.candidateKey(key) {
		return nil
	}
	if m.focusPreview {
		if m.page == detailPage && m.detailIdlePane != (paneIdentity{}) {
			switch key {
			case "enter":
				m.linkDetailUnassigned(m.selected())
				return nil
			case " ", "right", "r":
				return nil
			}
		}
		if key == " " && m.focus == 0 {
			if m.page == listPage && m.attention {
				groups, group := m.selectedGroup(false)
				if !groups[group.id] && m.paneSelection < len(m.livePanes) {
					return m.openTerminalPreview(m.livePanes[m.paneSelection])
				}
				return nil
			}
			if t := m.selected(); m.page == detailPage && t != nil && len(t.panes) != 0 {
				return m.openTerminalPreview(m.currentPane(t.panes[clamp(m.detailSelection, 0, len(t.panes)-1)]))
			}
		}
		if m.focus == 0 && m.onHomeBoard() {
			if cmd, handled := m.homePaneCommand(key); handled {
				return cmd
			}
		}
		if m.page == taskMenuPage {
			m.taskMenuKey(key)
			return nil
		}
		if m.page == detailPage || (m.page == listPage && !m.attention && m.focus == 0) {
			if key == "N" && !m.onCandidates() {
				return m.openTaskTab(m.selected())
			}
			if m.taskBoardKey(key) {
				return nil
			}
		}
		if m.page == linkPage && key != "?" {
			m.focusLinkKey(key)
			return nil
		}
		if m.page == detailPage && (key == "right" || key == "w") {
			t := m.selected()
			if t == nil {
				return nil
			}
			if key == "w" || len(t.panes) == 0 {
				if p, ok := m.urgentTaskPane(t); ok {
					m.showPane(p)
					return m.readPaneAgent("", false)
				}
			} else {
				m.showPane(m.currentPane(t.panes[clamp(m.detailSelection, 0, len(t.panes)-1)]))
				return m.readPaneAgent("", false)
			}
			return nil
		}
		if m.page == detailPage && (key == "l" || key == "r") {
			m.openFocusPicker(key == "r")
			return nil
		}
		if m.page == detailPage && key == "u" {
			if t := m.selected(); m.selectedPaneUnavailable(t) {
				m.unlinkTaskPane(t, m.detailSelection)
			}
			return nil
		}
		if m.page == listPage && !m.attention && key == "a" {
			m.focusBrowse()
			return nil
		}
		if m.page == listPage && !m.attention && key == "enter" && m.focus == 0 {
			if t := m.selected(); t != nil && (m.onHomeBoard() || len(t.panes) != 1) {
				m.showDetails()
				return nil
			}
		}
	}
	if !m.demo && key == "enter" && m.focus == 0 {
		if m.page == scenePage && m.scene != nil {
			return m.jump(m.currentPane(m.scene.pane))
		}
		if m.page == listPage {
			groups, group := m.selectedGroup(false)
			if groups[group.id] {
				delete(groups, group.id)
				return nil
			}
		}
		if m.page == listPage && m.attention && m.paneSelection < len(m.livePanes) {
			return m.jump(m.livePanes[m.paneSelection])
		}
		if t := m.selected(); t != nil && len(t.panes) != 0 &&
			(m.page == detailPage || (m.page == listPage && !m.attention)) {
			index := 0
			if m.page == detailPage {
				index = clamp(m.detailSelection, 0, len(t.panes)-1)
			}
			return m.jump(m.currentPane(t.panes[index]))
		}
	}
	scene := m.scene
	m.handle(key)
	if m.page == scenePage && m.scene != scene {
		return m.readPaneAgent("", false)
	}
	return nil
}

func (m *model) selected() *task {
	if m.taskSelection < 0 || m.taskSelection >= len(m.tasks) {
		return nil
	}
	return m.tasks[m.taskSelection]
}

func (m *model) handle(key string) {
	if key == "?" {
		m.cancelPaneTurn()
		m.page, m.helpOffset = helpPage, 0
		return
	}
	if m.page != listPage {
		m.handlePage(key)
		return
	}

	t := m.selected()
	if !m.jumping {
		m.notice = ""
	}
	switch key {
	case "v":
		if m.focusPreview && m.attention {
			order := [...]grouping{byStatus, bySession, byAgent, byHost}
			idx := slices.Index(order[:], m.grouping)
			m.grouping = order[(idx+1)%len(order)]
		} else if m.onHomeBoard() && m.focus == 0 {
			m.setHomeOverview(!m.homeOverview)
		} else {
			m.grouping = (m.grouping + 1) % 3
		}
	case "g":
		if !m.demo {
			m.hostFilter = ""
			m.refreshPanes()
		}
	case "tab":
		m.attention = !m.attention
		m.focus, m.peek = 0, false
	case "n":
		m.attention, m.focus = true, 0
		m.allPanes = false
		m.refreshPanes()
		if groups := m.paneGroups(); len(groups) > 0 {
			m.paneSelection = groups[0].firstVisibleRow()
		}
	case "+":
		m.editing, m.attention, m.focus = true, false, 0
		m.input.set("")
		m.candidatesFocused = false
	case "z":
		m.density = (m.density + 1) % len(densityLabels)
	case "f":
		m.focus = (m.focus + 1) % 3
	case "esc":
		if m.focus != 0 {
			m.focus = 0
			return
		}
	case "up", "k", "down", "j":
		delta := 1
		if key == "up" || key == "k" {
			delta = -1
		}
		m.move(delta)
	}
	if m.focus != 0 {
		if key == "enter" && m.focus == 1 && !m.demo && m.hostSelection < len(m.feed.Hosts) {
			m.showHost(m.hostSelection + 1)
		}
		return
	}
	groups, group := m.selectedGroup(key == "C")
	if key == "a" && m.attention {
		m.allPanes = !m.allPanes
		m.refreshPanes()
		return
	}
	if (key == "c" || key == "C") && groups != nil {
		toggleGroup(groups, group)
		m.peek = false
		return
	}
	if groups[group.id] {
		switch key {
		case "right", "enter":
			delete(groups, group.id)
		}
		return
	}
	if m.attention {
		if m.paneSelection < len(m.livePanes) {
			p := m.livePanes[m.paneSelection]
			switch key {
			case "right", "o":
				m.showPane(p)
			case " ":
				m.peek = !m.peek
			case "enter":
				if m.demo {
					m.showPane(p)
				}
			case "l":
				if t != nil && m.taskGroupFolded(t) {
					m.notice = m.text("先在任务页展开分组并选择任务")
				} else {
					m.link(t, p)
				}
			case "p":
				id := identityOf(p)
				if m.panePins[id] {
					delete(m.panePins, id)
				} else {
					m.panePins[id] = true
				}
			}
		}
		return
	}
	if t == nil {
		return
	}
	switch key {
	case "right":
		if m.expanded[t] {
			m.showDetails()
		} else {
			m.expanded[t] = true
		}
	case "left", "esc":
		delete(m.expanded, t)
	case "o":
		m.showDetails()
	case "enter":
		m.openTask(t)
	case " ":
		m.peek = !m.peek
	case "p":
		m.pin(t)
	case "b":
		m.color(t)
	}
}

func (m *model) handlePage(key string) {
	if key == "esc" || key == "left" {
		if m.page == scenePage {
			m.cancelPaneTurn()
			m.page = m.sceneBack
		} else {
			m.page = listPage
		}
		return
	}
	if m.page == helpPage {
		switch key {
		case "up", "k":
			m.helpOffset = max(0, m.helpOffset-1)
		case "down", "j":
			m.helpOffset++
		}
		return
	}
	if m.page == scenePage {
		switch key {
		case "d":
			m.discoverCandidate()
		case "w":
			m.toggleCandidateWatch()
		case "up", "k":
			m.scene.offset = max(0, m.scene.offset-1)
		case "down", "j":
			m.scene.offset++
		}
		return
	}
	if m.page != detailPage {
		return
	}
	if t := m.selected(); t != nil {
		last := len(t.panes) - 1
		switch key {
		case "up", "k", "down", "j":
			delta := 1
			if key == "up" || key == "k" {
				delta = -1
			}
			if m.focusPreview {
				m.moveDetailSelection(t, delta)
			} else {
				m.detailSelection = clamp(m.detailSelection+delta, 0, last)
			}
		case "s":
			m.cycleTaskState(t)
		case "p":
			m.pin(t)
		case "b":
			m.color(t)
		case "t":
			next := (t.kind + 1) % taskKind(len(kindLabels))
			if m.store == nil || m.saved(m.store.setKind(t.id, next)) {
				t.kind = next
			}
		case "enter":
			if m.demo && len(t.panes) > 0 {
				m.showPane(t.panes[m.detailSelection])
			}
		}
	}
}

func (m *model) move(delta int) {
	m.peek = false
	limit := resourceHeight(m.height)
	switch m.focus {
	case 1:
		if m.demo {
			m.hostOffset = clamp(m.hostOffset+delta, 0, len(hostExamples)-limit)
		} else {
			m.hostSelection = clamp(m.hostSelection+delta, 0, len(m.feed.Hosts)-1)
			m.hostOffset = clamp(m.hostOffset, max(0, m.hostSelection-limit+1), m.hostSelection)
		}
	case 2:
		count := 2
		if m.demo {
			count = len(quotaExamples)
		}
		m.quotaOffset = clamp(m.quotaOffset+delta, 0, count-limit/2)
	default:
		if m.onHomeBoard() {
			m.moveHome(delta)
		} else if m.attention {
			m.paneSelection = moveInGroups(m.paneGroups(), m.paneSelection, delta)
		} else {
			groups := m.taskGroups()
			next := moveInGroups(groups, m.taskSelection, delta)
			if m.focusPreview && len(groups) == 0 {
				next = -1
			}
			if delta > 0 && next == m.taskSelection && !m.archivedView {
				m.candidatesFocused, m.candidatePane = true, paneIdentity{}
			}
			m.taskSelection = next
		}
	}
}

func (m *model) edit(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.taskEdit != nil {
			m.page, m.taskEdit = m.taskEdit.back, nil
		}
		m.editing, m.input, m.candidateDraft = false, lineInput{}, nil
	case tea.KeyEnter:
		title := strings.TrimSpace(m.input.value)
		if m.taskEdit != nil {
			m.finishTaskEdit(title)
			return
		}
		if title != "" {
			if m.candidateDraft != nil {
				m.acceptCandidate(title)
				return
			}
			t := &task{id: m.nextTaskID(), title: title}
			if m.store != nil {
				id, err := m.store.addTask(title)
				if !m.saved(err) {
					return
				}
				t.id = id
			}
			m.tasks = append(m.tasks, t)
			m.selectTask(t)
			m.editing, m.input, m.archivedView = false, lineInput{}, false
			if m.focusPreview {
				m.showDetails()
			}
		}
	default:
		m.input.update(msg)
	}
}

func (m *model) pin(t *task) {
	if t.parentID != 0 || m.archivedTask(t) {
		m.notice = m.text("请在任务页置顶主任务")
		return
	}
	if m.store != nil && !m.saved(m.store.pin(t.id, !t.pinned)) {
		return
	}
	t.pinned = !t.pinned
	if m.focusPreview {
		m.sortTaskBoard(t)
		return
	}
	slices.SortStableFunc(m.tasks, func(a, b *task) int {
		if a.pinned == b.pinned {
			return 0
		}
		if a.pinned {
			return -1
		}
		return 1
	})
	m.taskSelection = slices.Index(m.tasks, t)
}

func (m *model) color(t *task) {
	next := (t.color + 1) % len(colorLabels)
	if m.store == nil || m.saved(m.store.color(t.id, storedColors[next])) {
		t.color = next
	}
}

func (m *model) saved(err error) bool {
	if err == nil {
		m.notice = ""
		return true
	}
	m.notice = m.text("保存失败 · ") + err.Error()
	return false
}

func (m *model) openTask(t *task) {
	if m.demo && len(t.panes) > 0 {
		m.showPane(t.panes[0])
		return
	}
	m.showDetails()
}

func (m *model) showDetails() {
	m.page, m.detailSelection, m.notice = detailPage, 0, ""
	m.detailIdlePane = paneIdentity{}
	if t := m.selected(); t != nil {
		m.touchTask(t)
	}
}

func (m *model) showPane(p rex.Pane) {
	m.cancelPaneTurn()
	m.scene, m.sceneBack, m.page = &paneScene{pane: p}, m.page, scenePage
	if m.store != nil && p.App == "codex" {
		m.scene.binding, _ = m.store.paneAgentBinding(p)
	}
	m.notice = ""
}

func (m *model) showHost(slot int) {
	if slot > len(m.feed.Hosts) {
		m.notice = m.text("没有这个编号的主机")
		return
	}
	m.cancelPaneTurn()
	m.hostFilter = ""
	if slot > 0 {
		m.hostFilter = m.feed.Hosts[slot-1].ID
	}
	m.page, m.attention, m.focus, m.peek = listPage, true, 0, false
	m.notice = ""
	m.refreshPanes()
}

func (m *model) inHostScope(p rex.Pane) bool {
	return m.hostFilter == "" || m.hostFilter == p.Host.ID
}

func (m *model) isAlert(p rex.Pane) bool {
	return p.NeedsAttention() && m.feed.HostState(p.Host.ID) != rex.Offline &&
		!m.seenResults[identityOf(p)].matches(p)
}

func (m *model) alertCount() int {
	count := 0
	for _, p := range m.feed.Panes {
		if m.inHostScope(p) && m.isAlert(p) {
			count++
		}
	}
	return count
}

type paneIdentity struct{ host, session, block string }

func identityOf(p rex.Pane) paneIdentity {
	return paneIdentity{p.Host.ID, p.SessionID, p.ID}
}

func samePane(a, b rex.Pane) bool {
	return identityOf(a) == identityOf(b)
}

func (m *model) refreshPanes() {
	var selected rex.Pane
	if m.paneSelection < len(m.livePanes) {
		selected = m.livePanes[m.paneSelection]
	}
	m.livePanes = nil
	for _, pane := range m.feed.Panes {
		if m.inHostScope(pane) && (m.allPanes || m.isAlert(pane)) {
			m.livePanes = append(m.livePanes, pane)
		}
	}
	found := slices.IndexFunc(m.livePanes, func(p rex.Pane) bool { return samePane(p, selected) })
	m.paneSelection = 0
	if found >= 0 {
		m.paneSelection = found
	} else if groups := m.paneGroups(); len(groups) != 0 {
		m.paneSelection = groups[0].firstVisibleRow()
	}
	m.hostSelection = clamp(m.hostSelection, 0, len(m.feed.Hosts)-1)
	m.hostOffset = clamp(m.hostOffset, max(0, m.hostSelection-resourceHeight(m.height)+1), m.hostSelection)
}

func (m *model) jump(target rex.Pane) tea.Cmd {
	if m.jumping {
		return nil
	}
	if m.feed.HostState(target.Host.ID) == rex.Offline {
		m.notice = m.text("目标主机失联，等待 Rex 重连")
		return nil
	}
	m.jumping, m.notice = true, m.text("正在打开 Rex 窗格…")
	ctx, location := m.ctx, m.rexContext
	return func() tea.Msg {
		return jumpResult{location.Focus(ctx, target)}
	}
}

func (m *model) currentPane(p rex.Pane) rex.Pane {
	if index := slices.IndexFunc(m.feed.Panes, func(live rex.Pane) bool { return samePane(live, p) }); index >= 0 {
		return m.feed.Panes[index]
	}
	return p
}

func (m *model) link(t *task, p rex.Pane) {
	if t == nil {
		m.notice = m.text("先在任务页新建或选择任务")
		return
	}
	if m.archivedTask(t) {
		m.notice = m.text("请先还原任务")
		return
	}
	index := slices.IndexFunc(t.panes, func(old rex.Pane) bool { return samePane(old, p) })
	if index >= 0 {
		m.unlinkTaskPane(t, index)
		return
	}
	if m.store != nil && !m.saved(m.store.bind(t.id, p, false)) {
		return
	}
	t.addBinding(p, paneBinding{source: linkManual, linkedAt: time.Now().UnixMilli()})
	delete(m.removedAgents, identityOf(p))
	m.touchTask(t)
	m.refreshCandidates()
	m.notice = m.text("已关联 · ") + m.taskText(t, t.title)
}

func (m *model) unlinkTaskPane(t *task, index int) {
	pane := t.panes[index]
	if m.store != nil && !m.saved(m.store.bind(t.id, pane, true)) {
		return
	}
	t.panes = slices.Delete(t.panes, index, index+1)
	delete(t.bindings, identityOf(pane))
	m.removedAgents[identityOf(pane)] = true
	if m.page == detailPage {
		m.detailSelection = clamp(index, 0, len(t.panes)-1)
	}
	m.touchTask(t)
	m.refreshCandidates()
	m.notice = m.text("已解除关联 · ") + m.taskText(t, t.title)
}

func (m *model) paneLocation(p rex.Pane) string {
	return strings.Join(nonEmpty(m.hostLabel(p.Host), m.sampleText(p.Session), appName(p.App)), " · ")
}

func nonEmpty(parts ...string) []string {
	return slices.DeleteFunc(parts, func(value string) bool { return value == "" })
}

func paneTitle(p rex.Pane) string {
	if title := reportedPaneTitle(p); title != "" {
		return title
	}
	return "终端窗格"
}

func reportedPaneTitle(p rex.Pane) string {
	label := usefulTitle(p.Label, p)
	if p.Window != "" && !generatedWindowLabel(p.Window, p.WindowID) {
		if label && p.Label != p.Window {
			return p.Window + " · " + p.Label
		}
		return p.Window
	}
	if label {
		return p.Label
	}
	for _, title := range []string{p.AgentTitle, p.TerminalTitle} {
		if usefulTitle(title, p) {
			return title
		}
	}
	if p.App != "" {
		return appName(p.App)
	}
	return ""
}

func generatedWindowLabel(label, id string) bool {
	short, ok := strings.CutPrefix(label, "Window ")
	return ok && len(short) == 8 && len(id) >= 8 && strings.EqualFold(short, id[len(id)-8:])
}

func usefulTitle(title string, p rex.Pane) bool {
	if generatedWindowLabel(title, p.WindowID) || strings.EqualFold(title, p.App) {
		return false
	}
	switch strings.ToLower(title) {
	case "", "shell", "terminal", "bash", "zsh", "fish", "sh", "powershell":
		return false
	}
	return true
}

func appName(app string) string {
	switch app {
	case "codex":
		return "Codex"
	case "claude-code":
		return "Claude Code"
	case "kimi-code":
		return "Kimi Code"
	}
	return app
}

func clamp(value, low, high int) int {
	return max(low, min(value, high))
}

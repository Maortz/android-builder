package build

import (
	"fmt"
	"io"
	"time"
)

type Phase string

const (
	PhaseTriggering   Phase = "triggering"
	PhaseWaitingStart Phase = "waiting"
	PhaseBuilding     Phase = "building"
	PhaseDownloading  Phase = "downloading"
)

type Progress struct {
	w           io.Writer
	buildID     string
	workflowURL string
	start       time.Time
}

func NewProgress(w io.Writer) *Progress { return &Progress{w: w, start: time.Now()} }

func (p *Progress) Start(buildID string) {
	p.buildID = buildID
	fmt.Fprintf(p.w, "android-builder — build %s\n\n", buildID)
}

func (p *Progress) Update(_ Phase, msg string) { fmt.Fprintf(p.w, "  ⏳ %s\n", msg) }

func (p *Progress) Complete(_ Phase, msg string) { fmt.Fprintf(p.w, "  ✅ %s\n", msg) }

func (p *Progress) Error(_ Phase, err error) { fmt.Fprintf(p.w, "  ❌ %v\n", err) }

func (p *Progress) SetWorkflowURL(u string) {
	p.workflowURL = u
	fmt.Fprintf(p.w, "  🔗 %s\n", u)
}

func (p *Progress) UpdateDownloadProgress(downloaded, total int64) {
	if total > 0 {
		fmt.Fprintf(p.w, "\r  ⬇️  %.0f%%", float64(downloaded)/float64(total)*100)
	}
}

func (p *Progress) Finish() {
	fmt.Fprintf(p.w, "\n\nDone in %s\n", time.Since(p.start).Round(time.Second))
}

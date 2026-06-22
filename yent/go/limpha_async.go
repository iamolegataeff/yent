package yent

// limpha_async.go — between-turn circulation for the shared limpha brain.
//
// The synchronous Store/StoreSeam API stays intact for deterministic tests and
// one-shot tools. Long-running inference can enable this worker so body turns do
// not block on SQLite writes; a dual-body turn is still persisted atomically as
// conversation -> seam with the seam linked to the stored conversation id.

type limphaJobKind int

const (
	limphaJobTurn limphaJobKind = iota
	limphaJobSeam
)

type limphaJob struct {
	kind     limphaJobKind
	prompt   string
	response string
	state    LimphaState
	seam     *Seam
}

type limphaAsync struct {
	queue chan limphaJob
	stop  chan struct{}
	done  chan struct{}
}

// StartAsync starts a single writer that drains memory writes between turns.
// Calling it twice is a no-op. buffer <= 0 uses a conservative default.
func (c *LimphaClient) StartAsync(buffer int) {
	if c == nil || !c.connected {
		return
	}
	if buffer <= 0 {
		buffer = 256
	}
	c.asyncMu.Lock()
	if c.async != nil {
		c.asyncMu.Unlock()
		return
	}
	a := &limphaAsync{
		queue: make(chan limphaJob, buffer),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	c.async = a
	c.asyncMu.Unlock()
	go c.runAsync(a)
}

// StopAsync stops the writer after draining all queued jobs.
func (c *LimphaClient) StopAsync() {
	if c == nil {
		return
	}
	c.asyncMu.Lock()
	a := c.async
	if a == nil {
		c.asyncMu.Unlock()
		return
	}
	c.async = nil
	close(a.stop)
	c.asyncMu.Unlock()
	<-a.done
}

func (c *LimphaClient) runAsync(a *limphaAsync) {
	defer close(a.done)
	for {
		select {
		case job := <-a.queue:
			c.runLimphaJob(job)
		case <-a.stop:
			for {
				select {
				case job := <-a.queue:
					c.runLimphaJob(job)
				default:
					return
				}
			}
		}
	}
}

func (c *LimphaClient) runLimphaJob(job limphaJob) {
	if c == nil || !c.connected {
		return
	}
	switch job.kind {
	case limphaJobTurn:
		convID, err := c.store(job.prompt, job.response, job.state)
		if err != nil || job.seam == nil {
			return
		}
		seam := *job.seam
		seam.ConversationID = convID
		_, _ = c.StoreSeam(seam)
	case limphaJobSeam:
		if job.seam != nil {
			_, _ = c.StoreSeam(*job.seam)
		}
	}
}

// EnqueueTurn queues one conversation write. If seam is non-nil, the worker
// stores the seam after the conversation and links conversation_id.
func (c *LimphaClient) EnqueueTurn(prompt, response string, state LimphaState, seam *Seam) bool {
	return c.enqueueLimphaJob(limphaJob{
		kind:     limphaJobTurn,
		prompt:   prompt,
		response: response,
		state:    state,
		seam:     cloneSeam(seam),
	})
}

// EnqueueSeam queues a seam that is not tied to a just-stored conversation.
func (c *LimphaClient) EnqueueSeam(seam Seam) bool {
	return c.enqueueLimphaJob(limphaJob{kind: limphaJobSeam, seam: cloneSeam(&seam)})
}

func (c *LimphaClient) enqueueLimphaJob(job limphaJob) bool {
	if c == nil {
		return false
	}
	c.asyncMu.Lock()
	defer c.asyncMu.Unlock()
	if c.async == nil || !c.connected {
		return false
	}
	select {
	case c.async.queue <- job:
		return true
	default:
		return false
	}
}

// AsyncBacklog exposes the current queued job count for diagnostics.
func (c *LimphaClient) AsyncBacklog() int {
	if c == nil {
		return 0
	}
	c.asyncMu.Lock()
	defer c.asyncMu.Unlock()
	if c.async == nil {
		return 0
	}
	return len(c.async.queue)
}

func (c *LimphaClient) asyncEnabled() bool {
	if c == nil {
		return false
	}
	c.asyncMu.Lock()
	defer c.asyncMu.Unlock()
	return c.async != nil && c.connected
}

func cloneSeam(seam *Seam) *Seam {
	if seam == nil {
		return nil
	}
	cp := *seam
	return &cp
}

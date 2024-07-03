package controllerservice

import (
	"context"
	"log/slog"
	"sprinkler-controller-service/internal/config"
	"sync"
	"time"
)

type Task struct {
	ZoneName string
	Action   bool
}

type ControllerService struct {
	Ctx           context.Context
	Wg            *sync.WaitGroup
	Logger        *slog.Logger
	Config        *config.Config
	ApiHandler    IApiHandler
	LastResetDate time.Time
	Mutex         sync.RWMutex
	TaskQueue     chan *config.ScheduleItem
}

func NewControllerService(ctx context.Context,
	wg *sync.WaitGroup,
	logger *slog.Logger,
	cfg *config.Config,
	apiHdnlr IApiHandler,
) *ControllerService {
	return &ControllerService{
		Ctx:           ctx,
		Wg:            wg,
		Logger:        logger,
		Config:        cfg,
		ApiHandler:    apiHdnlr,
		LastResetDate: time.Now(),
		Mutex:         sync.RWMutex{},
		TaskQueue:     make(chan *config.ScheduleItem, 100),
	}
}

func (c *ControllerService) ResetIfNewDay() {
	currentDate := time.Now().Truncate(24 * time.Hour)
	if currentDate.After(c.LastResetDate) {
		for _, zoneItem := range c.Config.ZoneList {
			for idx := range zoneItem.Schedule {
				c.Mutex.Lock()
				zoneItem.Schedule[idx].Completed = false
				c.Mutex.Unlock()
			}
		}
		c.LastResetDate = currentDate
	}
}

func (c *ControllerService) Run() {
	taskProcCtx, taskProcCancel := context.WithCancel(context.Background())
	taskProcWg := sync.WaitGroup{}
	taskProcWg.Add(1)

	go c.TaskProcessor(&taskProcWg, taskProcCtx)

	for {
		select {
		case <-c.Ctx.Done():
			c.Logger.Info("Done context signal detected in controller service - cleaning up")
			taskProcCancel()
			taskProcWg.Wait()
			c.Wg.Done()
			return
		default:

			c.ResetIfNewDay()

			c.Logger.Info("Checking zone schedule...")
			for zoneName, zoneInfo := range c.Config.ZoneList {
				c.Logger.Debug("Checking zone schedule", "zone", zoneName)
				for idx := range zoneInfo.Schedule {
					currentTime := time.Now()

					c.Logger.Debug("Comparing current and start times", "zone", zoneName, "current", currentTime, "startTime", zoneInfo.Schedule[idx].StartTime)

					startTime, err := time.Parse(time.TimeOnly, zoneInfo.Schedule[idx].StartTime)
					if err != nil {
						c.Logger.Error("Error parsing start time", "startTime", zoneInfo.Schedule[idx].StartTime, "error", err)
					}

					duration := time.Duration(zoneInfo.Schedule[idx].DurationMinutes)
					endTime := startTime.Add(duration * time.Minute)

					c.Mutex.RLock()
					if currentTime.After(startTime) && !zoneInfo.Schedule[idx].Active {
						c.Logger.Debug("Zone is not active and current time exceeds start time for zone schedule item", "zone", zoneName, "currentTime", time.Now(), "startTime", startTime)
						c.Logger.Info("Starting sprinkler event", "zoneName", zoneName, "currentTime", currentTime, "startTime", zoneInfo.Schedule[idx].StartTime, "endTime", endTime, "durationMinutes", zoneInfo.Schedule[idx].DurationMinutes)
						c.TaskQueue <- &zoneInfo.Schedule[idx]
					}

					if currentTime.After(endTime) && zoneInfo.Schedule[idx].Active {
						c.Logger.Debug("Zone is active and current time exceeds end time for zone schedule item", "zone", zoneName, "currentTime", time.Now(), "startTime", startTime)
						c.Logger.Info("Stopping sprinkler event", "zoneName", zoneName, "currentTime", currentTime, "startTime", zoneInfo.Schedule[idx].StartTime, "endTime", endTime, "durationMinutes", zoneInfo.Schedule[idx].DurationMinutes)
						c.TaskQueue <- &zoneInfo.Schedule[idx]
					}
					c.Mutex.RUnlock()
				}
			}
		}

		time.Sleep(1 * time.Second)
	}
}

func (c *ControllerService) TaskProcessor(wg *sync.WaitGroup, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.Logger.Info("Done signal detected in TaskProcessor - exiting")
			wg.Done()
			return
		case t := <-c.TaskQueue:
			// Send POST request to API to start the event
			err := c.ApiHandler.SendSprinklerEventRequest(t)
			if err != nil {
				c.Logger.Error("API request error", "event", t)
				continue
			}
			c.Mutex.Lock()
			t.Mutex.Lock()
			t.Active = true
			t.Mutex.Unlock()
			c.Mutex.Unlock()
		}
	}
}

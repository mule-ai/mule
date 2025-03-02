package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestAddTaskWithValidSchedule(t *testing.T) {
	logger := logr.Discard()
	scheduler := NewScheduler(logger)
	taskKey := "test-task"
	schedule := "* * * * *" // Every minute
	action := func() {}
	err := scheduler.AddTask(taskKey, schedule, action)
	if err != nil {
		t.Errorf("Expected no error when adding a task with a valid schedule but got: %v", err)
	}

	if _, exists := scheduler.tasks[taskKey]; !exists {
		t.Errorf("Task was not added to the tasks map")
	}
}

func TestAddTaskWithInvalidSchedule(t *testing.T) {
	logger := logr.Discard()
	scheduler := NewScheduler(logger)
	taskKey := "test-task"
	schedule := "invalid-cron-expression" // Invalid cron schedule
	action := func() {}
	err := scheduler.AddTask(taskKey, schedule, action)
	if err == nil {
		t.Errorf("Expected an error when adding a task with an invalid schedule but got none")
	}
}

func TestAddExistingTaskWithNewSchedule(t *testing.T) {
	logger := logr.Discard()
	scheduler := NewScheduler(logger)
	taskKey := "test-task"
	schedule1 := "* * * * *" // Every minute
	schedule2 := "0 0 * * *" // Midnight every day
	action := func() {}

	err := scheduler.AddTask(taskKey, schedule1, action)
	if err != nil {
		t.Errorf("Expected no error when adding a task with a valid schedule but got: %v", err)
	}

	err = scheduler.AddTask(taskKey, schedule2, action)
	if err != nil {
		t.Errorf("Expected no error when updating an existing task with a new schedule but got: %v", err)
	}

	task := scheduler.tasks[taskKey]
	if task.Schedule != schedule2 {
		t.Errorf("Task schedule was not updated correctly. Expected %v, got %v", schedule2, task.Schedule)
	}
}

func TestRemoveTask(t *testing.T) {
	logger := logr.Discard()
	scheduler := NewScheduler(logger)
	taskKey := "test-task"
	schedule := "* * * * *" // Every minute
	action := func() {}
	err := scheduler.AddTask(taskKey, schedule, action)
	if err != nil {
		t.Errorf("Expected no error when adding a task with a valid schedule but got: %v", err)
	}

	scheduler.RemoveTask(taskKey)

	if _, exists := scheduler.tasks[taskKey]; exists {
		t.Errorf("Task was not removed from the tasks map")
	}
}

func TestUpdateTask(t *testing.T) {
	logger := logr.Discard()
	scheduler := NewScheduler(logger)
	taskKey := "test-task"
	schedule1 := "* * * * *" // Every minute
	schedule2 := "0 0 * * *" // Midnight every day
	action := func() {}

	err := scheduler.AddTask(taskKey, schedule1, action)
	if err != nil {
		t.Errorf("Expected no error when adding a task with a valid schedule but got: %v", err)
	}

	err = scheduler.UpdateTask(taskKey, schedule2, action)
	if err != nil {
		t.Errorf("Expected no error when updating an existing task with a new schedule but got: %v", err)
	}

	task := scheduler.tasks[taskKey]
	if task.Schedule != schedule2 {
		t.Errorf("Task schedule was not updated correctly. Expected %v, got %v", schedule2, task.Schedule)
	}
}

func TestSchedulerConcurrency(t *testing.T) {
	logger := logr.Discard()
	scheduler := NewScheduler(logger)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			taskKey := "test-task-" + string(rune('a'+i))
			schedule := "* * * * *" // Every minute
			action := func() {}

			err := scheduler.AddTask(taskKey, schedule, action)
			if err != nil {
				t.Errorf("Expected no error when adding a task with a valid schedule but got: %v", err)
			}

			scheduler.RemoveTask(taskKey)
		}(i)
	}

	wg.Wait()

	time.Sleep(1 * time.Second) // Ensure all tasks have been processed
}

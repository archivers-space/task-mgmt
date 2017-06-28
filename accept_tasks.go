package main

import (
	"encoding/json"
	"fmt"
	"github.com/archivers-space/task-mgmt/defs/ipfs"
	"github.com/archivers-space/task-mgmt/tasks"
	"github.com/streadway/amqp"
)

// taskdefs is a map of all possible task names to their respective "New" funcs
var taskdefs = map[string]tasks.NewTaskFunc{
	"ipfs.add": ipfs.NewTaskAdd,
}

// start accepting tasks, if setup doesn't error, it returns a stop channel
// writing to stop will teardown the func and stop accepting tasks
func acceptTasks() (stop chan bool, err error) {
	stop = make(chan bool)

	conn, err := amqp.Dial(cfg.AmqpUrl)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to RabbitMQ: %s", err.Error())
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("Failed to open a channel: %s", err.Error())
	}

	q, err := ch.QueueDeclare(
		"tasks", // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("Error declaring que: %s", err.Error())
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("", err)
	}

	go func() {
		for d := range msgs {
			newTask := taskdefs[d.Type]
			if newTask == nil {
				log.Errorf("unknown task type: %s", d.Type)
				d.Nack(false, false)
				continue
			}

			task := newTask()
			if err := json.Unmarshal(d.Body, task); err != nil {
				log.Errorf("error decoding task body: %s", err.Error())
				d.Nack(false, false)
				continue
			}

			pc := make(chan tasks.Progress, 10)
			task.Do(pc)
			for p := range pc {
				if p.Error != nil {
					log.Errorf("task error: %s", err.Error())
					d.Nack(false, false)
					break
				}
				if p.Done {
					log.Infof("completed task: %s, %s", d.MessageId, d.Type)
					d.Ack(false)
					break
				}
			}
		}
		<-stop
		ch.Close()
		conn.Close()
	}()

	return stop, nil
}
package rpc

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/rabbitmq/amqp091-go"
	"waki.mobi/go-rabbitmq/broker"
)

func main() {
	conn, ch, err := broker.RabbitMQ()
	if err != nil {
		panic(err)
	}

	defer func() {
		ch.Close()
		conn.Close()
	}()

	q, err := ch.QueueDeclare("rpc_queue", false, false, false, false, nil)
	if err != nil {
		panic(errors.Wrap(err, "Failed to declare queue"))
	}

	err = ch.Qos(1, 0, false)
	if err != nil {
		panic(errors.Wrap(err, "Failed to set qos"))
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		panic(errors.Wrap(err, "Failed to consume"))
	}

	forever := make(chan struct{})

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Username: "",
		Password: "",
		DB:       0,
	})

	ctx := context.Background()

	go func() {
		for d := range msgs {
			log.Println("Processing message :", string(d.Body))
			response, err := rdb.Get(ctx, string(d.Body)).Result()

			if errors.Is(err, redis.Nil) {
				t := time.Duration(len(d.Body))
				time.Sleep(t * time.Second)

				result := isPalindrome(string(d.Body))
				response = strconv.FormatBool(result)
				err = rdb.Set(ctx, string(d.Body), response, 10*time.Second).Err()
				if err != nil {
					panic(errors.Wrap(err, "Failed to set redis"))
				}
			}

			if err != nil {
				panic(errors.Wrap(err, "Failed to get cache"))
			}

			err = ch.Publish("", d.ReplyTo, false, false, amqp091.Publishing{
				ContentType:   "text/plain",
				CorrelationId: d.CorrelationId,
				Body:          []byte(response),
			})
			if err != nil {
				panic(errors.Wrap(err, "Failed to publish message"))
			}
			d.Ack(false)
		}
	}()

	fmt.Println("Waiting for RPC messages")
	<-forever
}

func isPalindrome(s string) bool {
	for i := 0; i < len(s)/2; i++ {
		if s[i] != s[len(s)-1-i] {
			return false
		}
	}
	return true
}

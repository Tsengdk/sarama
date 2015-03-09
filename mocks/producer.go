package mocks

import (
	"errors"
	"sync"

	"github.com/Shopify/sarama"
)

// A simple interafce that includes the testing.T methods we use to report
// expectation violations when using the mock objects.
type TestReporter interface {
	Errorf(string, ...interface{})
}

var (
	errProduceSuccess    error = nil
	errOutOfExpectations       = errors.New("No more expectations set on mock producer")
)

type producerExpectation struct {
	Result error
}

// Producer implements sarama's Producer interface for testing purposes.
// Before you can send messages to it's Input channel, you have to set expectations
// so it knows how to handle the input. This way you can easily test success and
// failure scenarios.
type Producer struct {
	l            sync.Mutex
	expectations []*producerExpectation
	closed       chan struct{}
	input        chan *sarama.ProducerMessage
	successes    chan *sarama.ProducerMessage
	errors       chan *sarama.ProducerError
}

// NewProducer instantiates a new Producer mock. The t argument should
// be the *testing.T instance of your test method. An error will be written to it if
// an expectation is violated. The config argument is used to determine whether it
// should ack successes on the Successes channel.
func NewProducer(t TestReporter, config *sarama.Config) *Producer {
	if config == nil {
		config = sarama.NewConfig()
	}
	mp := &Producer{
		closed:       make(chan struct{}, 0),
		expectations: make([]*producerExpectation, 0),
		input:        make(chan *sarama.ProducerMessage, config.ChannelBufferSize),
		successes:    make(chan *sarama.ProducerMessage, config.ChannelBufferSize),
		errors:       make(chan *sarama.ProducerError, config.ChannelBufferSize),
	}

	go func() {
		defer func() {
			close(mp.successes)
			close(mp.errors)
		}()

		for msg := range mp.input {
			mp.l.Lock()
			if mp.expectations == nil || len(mp.expectations) == 0 {
				mp.expectations = nil
				t.Errorf("No more expectation set on this mock producer to handle the input message.")
			} else {
				expectation := mp.expectations[0]
				mp.expectations = mp.expectations[1:]
				if expectation.Result == errProduceSuccess {
					if config.Producer.AckSuccesses {
						mp.successes <- msg
					}
				} else {
					mp.errors <- &sarama.ProducerError{Err: expectation.Result, Msg: msg}
				}
			}
			mp.l.Unlock()
		}

		mp.l.Lock()
		if len(mp.expectations) > 0 {
			t.Errorf("Expected to exhaust all expectations, but %d are left.", len(mp.expectations))
		}
		mp.l.Unlock()

		close(mp.closed)
	}()

	return mp
}

////////////////////////////////////////////////
// Implement Producer interface
////////////////////////////////////////////////

// AsyncClose corresponds with the AsyncClose method of sarama's Producer implementation.
// By closing a mock producer, you also tell it that no more input will be provided, so it will
// write an error to the test state if there's any remaining expectations.
func (mp *Producer) AsyncClose() {
	close(mp.input)
}

// Close corresponds with the Close method of sarama's Producer implementation.
// By closing a mock producer, you also tell it that no more input will be provided, so it will
// write an error to the test state if there's any remaining expectations.
func (mp *Producer) Close() error {
	mp.AsyncClose()
	<-mp.closed
	return nil
}

// Input corresponds with the Input method of sarama's Producer implementation.
// You have to set expectations on the mock producer before writing messages to the Input
// channel, so it knows how to handle them. If there is no more remaining expectations and
// a messages is written to the Input channel, the mock producer will write an error to the test
// state object.
func (mp *Producer) Input() chan<- *sarama.ProducerMessage {
	return mp.input
}

// Successes corresponds with the Successes method of sarama's Producer implementation.
func (mp *Producer) Successes() <-chan *sarama.ProducerMessage {
	return mp.successes
}

// Errors corresponds with the Errors method of sarama's Producer implementation.
func (mp *Producer) Errors() <-chan *sarama.ProducerError {
	return mp.errors
}

////////////////////////////////////////////////
// Setting expectations
////////////////////////////////////////////////

// ExpectInputAndSucceed sets an expectation on the mock producer that a message will be provided
// on the input channel. The mock producer will handle the message as if it is produced successfully,
// i.e. it will make it available on the Successes channel if the Producer.AckSuccesses setting
// is set to true.
func (mp *Producer) ExpectInputAndSucceed() {
	mp.l.Lock()
	defer mp.l.Unlock()
	mp.expectations = append(mp.expectations, &producerExpectation{Result: errProduceSuccess})
}

// ExpectInputAndFail sets an expectation on the mock producer that a message will be provided
// on the input channel. The mock producer will handle the message as if it failed to produce
// successfully. This means it will make a ProducerError available on the Errors channel.
func (mp *Producer) ExpectInputAndFail(err error) {
	mp.l.Lock()
	defer mp.l.Unlock()
	mp.expectations = append(mp.expectations, &producerExpectation{Result: err})
}

/**
*  @file
*  @copyright defined in go-seele/LICENSE
 */

package listener

import (
	"encoding/hex"
	"strings"

	"github.com/seeleteam/go-seele/accounts/abi"
	"github.com/seeleteam/go-seele/common/errors"
	"github.com/seeleteam/go-seele/log"
)

var (
	// ErrEventABILoadFailed is returned when abi string can't be loaded
	ErrEventABILoadFailed = errors.New("abi load failed")
)

var listenLog = log.GetLogger("listener")

// Listener represents subchain event listener.
type Listener struct {
	cfg       *config
	abiParser map[AbiTopic]abiParser
	log       *log.SeeleLog
}

type AbiTopic struct {
	Topic string
	Abi   string
}

type abiParser struct {
	MethodName string
	Parser     abi.ABI
}

// NewListener returns an initialized Listener.
func NewListener(ref map[string]string) *Listener {
	return &Listener{
		cfg:       newConfig(ref),
		abiParser: make(map[AbiTopic]abiParser),
		log:       listenLog,
	}
}

// GetABIParser parse the event config to abiParser.
func (l *Listener) GetABIParser() error {
	cfg := l.cfg

	abiParserMp := make(map[AbiTopic]abiParser)
	for _, event := range cfg.events {
		parser, err := abi.JSON(strings.NewReader(event.abi))
		if err != nil {
			l.log.Error("read abi error, %s, %s", event.methodName, err.Error())
			return ErrEventABILoadFailed
		}

		var abiParser abiParser
		abiParser.MethodName = event.methodName
		abiParser.Parser = parser

		topic := parser.Events[event.methodName].Id().Hex()
		// todo: If there is a method of the same name. the key of map should be topic := event.Topic + event.AbiPath
		key := AbiTopic{Topic: topic, Abi: event.abi}
		abiParserMp[key] = abiParser
	}

	l.abiParser = abiParserMp

	return nil
}

// Receipt represents an instance receipt from main chain.
type Receipt struct {
	Failed bool
	Logs   []*Log
}

// Log represents an log instance from main chain.
type Log struct {
	Address string
	Data    []string
	Topic   string
}

// Event represents a subchain contract event instance from Log.
type Event struct {
	At         AbiTopic
	MethodName string
	Arguments  []interface{}
}

type logGroup map[string][]*Log

// GroupByTopic converts the logs of receipts to map[string][]*Log.
func GroupByTopic(receipts []*Receipt) logGroup {
	if len(receipts) == 0 {
		return nil
	}

	logGroup := make(logGroup)
	for _, receipt := range receipts {
		if receipt.Failed == true || len(receipt.Logs) == 0 {
			continue
		}

		for _, log := range receipt.Logs {
			logGroup[log.Topic] = append(logGroup[log.Topic], log)
		}
	}

	return logGroup
}

// Filter converts Log to Event.
func (l *Listener) Filter(lg logGroup) []Event {
	var (
		events []Event
		err    error
	)
	for at, parser := range l.abiParser {
		logs, ok := lg[at.Topic]
		if !ok || logs == nil {
			continue
		}

		for _, log := range logs {
			var event Event

			if len(log.Data) != 0 {
				args := make([]string, len(log.Data))
				for i, data := range log.Data {
					args[i] = data[2:]
				}

				b, _ := hex.DecodeString(strings.Join(args, ""))

				event.Arguments, err = parser.Parser.Events[parser.MethodName].Inputs.UnpackValues(b)
				if err != nil {
					l.log.Warn("abi decode input args failed, %s", err.Error())
					continue
				}
			}

			event.At = at
			event.MethodName = parser.MethodName
			events = append(events, event)
		}
	}

	return events
}

package rule

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/srs"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/logger"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/x/list"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/pause"

	"go4.org/netipx"
)

var _ adapter.RuleSet = (*RemoteRuleSet)(nil)

type RemoteRuleSet struct {
	ctx            context.Context
	cancel         context.CancelFunc
	logger         logger.ContextLogger
	outbound       adapter.OutboundManager
	options        option.RuleSet
	updateInterval time.Duration
	dialer         N.Dialer
	access         sync.RWMutex
	rules          []adapter.HeadlessRule
	metadata       adapter.RuleSetMetadata
	lastUpdated    time.Time
	lastEtag       string
	updateTicker   *time.Ticker
	cacheFile      adapter.CacheFile
	pauseManager   pause.Manager
	callbacks      list.List[adapter.RuleSetUpdateCallback]
	refs           atomic.Int32
}

func NewRemoteRuleSet(ctx context.Context, logger logger.ContextLogger, options option.RuleSet) *RemoteRuleSet {
	ctx, cancel := context.WithCancel(ctx)
	var updateInterval time.Duration
	if options.RemoteOptions.UpdateInterval > 0 {
		updateInterval = time.Duration(options.RemoteOptions.UpdateInterval)
	} else {
		updateInterval = 24 * time.Hour
	}
	return &RemoteRuleSet{
		ctx:            ctx,
		cancel:         cancel,
		outbound:       service.FromContext[adapter.OutboundManager](ctx),
		logger:         logger,
		options:        options,
		updateInterval: updateInterval,
		pauseManager:   service.FromContext[pause.Manager](ctx),
	}
}

func (s *RemoteRuleSet) Name() string {
	return s.options.Tag
}

func (s *RemoteRuleSet) String() string {
	return strings.Join(F.MapToString(s.rules), " ")
}

func (s *RemoteRuleSet) StartContext(ctx context.Context, startContext *adapter.HTTPStartContext) error {
	return E.New("remote rule-sets are not compiled into this build: ", s.options.Tag)
}

func (s *RemoteRuleSet) PostStart() error {
	return nil
}

func (s *RemoteRuleSet) Metadata() adapter.RuleSetMetadata {
	s.access.RLock()
	defer s.access.RUnlock()
	return s.metadata
}

func (s *RemoteRuleSet) ExtractIPSet() []*netipx.IPSet {
	s.access.RLock()
	defer s.access.RUnlock()
	return common.FlatMap(s.rules, extractIPSetFromRule)
}

func (s *RemoteRuleSet) IncRef() {
	s.refs.Add(1)
}

func (s *RemoteRuleSet) DecRef() {
	if s.refs.Add(-1) < 0 {
		panic("rule-set: negative refs")
	}
}

func (s *RemoteRuleSet) Cleanup() {
	if s.refs.Load() == 0 {
		s.rules = nil
	}
}

func (s *RemoteRuleSet) RegisterCallback(callback adapter.RuleSetUpdateCallback) *list.Element[adapter.RuleSetUpdateCallback] {
	s.access.Lock()
	defer s.access.Unlock()
	return s.callbacks.PushBack(callback)
}

func (s *RemoteRuleSet) UnregisterCallback(element *list.Element[adapter.RuleSetUpdateCallback]) {
	s.access.Lock()
	defer s.access.Unlock()
	s.callbacks.Remove(element)
}

func (s *RemoteRuleSet) loadBytes(content []byte) error {
	var (
		ruleSet option.PlainRuleSetCompat
		err     error
	)
	switch s.options.Format {
	case C.RuleSetFormatSource:
		ruleSet, err = json.UnmarshalExtended[option.PlainRuleSetCompat](content)
		if err != nil {
			return err
		}
	case C.RuleSetFormatBinary:
		ruleSet, err = srs.Read(bytes.NewReader(content), false)
		if err != nil {
			return err
		}
	default:
		return E.New("unknown rule-set format: ", s.options.Format)
	}
	plainRuleSet, err := ruleSet.Upgrade()
	if err != nil {
		return err
	}
	rules := make([]adapter.HeadlessRule, len(plainRuleSet.Rules))
	for i, ruleOptions := range plainRuleSet.Rules {
		rules[i], err = NewHeadlessRule(s.ctx, ruleOptions)
		if err != nil {
			return E.Cause(err, "parse rule_set.rules.[", i, "]")
		}
	}
	s.access.Lock()
	s.metadata.ContainsProcessRule = HasHeadlessRule(plainRuleSet.Rules, isProcessHeadlessRule)
	s.metadata.ContainsWIFIRule = HasHeadlessRule(plainRuleSet.Rules, isWIFIHeadlessRule)
	s.metadata.ContainsIPCIDRRule = HasHeadlessRule(plainRuleSet.Rules, isIPCIDRHeadlessRule)
	s.rules = rules
	callbacks := s.callbacks.Array()
	s.access.Unlock()
	for _, callback := range callbacks {
		callback(s)
	}
	return nil
}

func (s *RemoteRuleSet) Close() error {
	s.rules = nil
	s.cancel()
	if s.updateTicker != nil {
		s.updateTicker.Stop()
	}
	return nil
}

func (s *RemoteRuleSet) Match(metadata *adapter.InboundContext) bool {
	for _, rule := range s.rules {
		if rule.Match(metadata) {
			return true
		}
	}
	return false
}

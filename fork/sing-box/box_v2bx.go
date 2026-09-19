package box

import "github.com/sagernet/sing-box/log"

func (s *Box) LogFactory() log.Factory {
	return s.logFactory
}

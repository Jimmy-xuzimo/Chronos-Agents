package protocol

func (p Policy) Validate() bool { return p.Interval >= 10 && p.Interval <= 86400 }

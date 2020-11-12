// Copyright 2017 David Ackroyd. All Rights Reserved.
// See LICENSE for licensing terms.

//整份代码都是从 grpc_recovery 拷贝过来的,为了能在回调的地方把 context 传递过去

package recovery

var (
	defaultOptions = &options{
		recoveryHandlerFunc: nil,
	}
)

type options struct {
	recoveryHandlerFunc RecoveryHandlerFunc
}

func evaluateOptions(opts []Option) *options {
	optCopy := &options{}
	*optCopy = *defaultOptions
	for _, o := range opts {
		o(optCopy)
	}
	return optCopy
}

type Option func(*options)

// WithRecoveryHandler customizes the function for recovering from a panic.
func WithRecoveryHandler(f RecoveryHandlerFunc) Option {
	return func(o *options) {
		o.recoveryHandlerFunc = f
	}
}

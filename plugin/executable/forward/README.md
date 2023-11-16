Starting from v0.58.0, the upstream module removes ServerIPAddrs, introducing breaking change to us. 

> **Added**
>
>    The `upstream.StaticResolver`, `upstream.ConsequentResolver`, and `upstream.ParallelResolver` implementations of `upstream.Resolver`.
> 
> **Changed**
>
>    The field `upstream.Options.Bootstrap` now has a type of `upstream.Resolver` instead of a string slice.
>    The `upstream.NewUpstreamResolver` now returns an `upstream.UpstreamResolver` instead of `upstream.Resolver`.
>    The `proxyutil.IPFromRR` now returns `netip.Addr` instead of `net.IP`.
> 
> **Removed**
> 
>    The field `upstream.Options.ServerIPAddrs`. Set `upstream.StaticResolver` into `upstream.Options.Bootstrap` instead.

If we try to adapt to the change, we can only use one ip for every upstream. Considering no security fixes are related, we will stick to the last version before v0.58 which is v0.57.3.
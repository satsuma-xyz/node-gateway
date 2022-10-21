package route

type RouterCollection struct {
	Routers []Router
}

func (r *RouterCollection) Start() {
	for _, router := range r.Routers {
		router.Start()
	}
}

func (r *RouterCollection) IsInitialized() bool {
	for _, router := range r.Routers {
		if !router.IsInitialized() {
			return false
		}
	}

	return true
}

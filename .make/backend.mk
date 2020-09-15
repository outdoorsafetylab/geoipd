CONFIG := local

backend: $(WATCHER)
	$(WATCHER) -c $(CONFIG)

clean/backend:

.PHONY: backend clean/backend

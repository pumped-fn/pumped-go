import { provide, derive, mutable, effect, resource } from "@pumped-fn/core"

const config = provide(() => mutable({
	increment: 1,
	interval: 1000
}))

const configController = provide((scope) => {
	return {
		changeIncrement: (increment: number) => scope.update(config, (config) => ({ ...config, increment })),
		changeInterval: (interval: number) => scope.update(config, (config) => ({ ...config, interval }))
	}
})

const counter = provide(() => mutable(0))

const timer = derive(
	[config],
	([config], scope) => {
		const interval = setInterval(() => {
			scope.update(counter, v => v + config.get().increment)
		}, config.get().interval)

		return effect(() => {
			console.log('cleanup')
			clearInterval(interval)
		})
	})

export const counterApp = {
	config,
	configController,
	counter,
	timer
}
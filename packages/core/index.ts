import {
	createScope,
	provide,
	derive,
	resource,
	mutable,
	effect,
} from "./src/core/core";

async function main() {
	const scope = createScope();

	const value = provide(() => mutable(1));
	const stringValue = provide(() => mutable("hello"));

	const double = derive([value, stringValue], ([value]) => value.get() * 2);
	const triple = derive([value], ([value]) => value.get() * 3);
	const resourceValue = derive([triple], ([triple]) => {
		return resource(triple, () => {
			console.log("cleanup", triple.get());
		});
	});

	const resolvedDouble = await scope.resolve(double);
	const resolvedTriple = await scope.resolve(triple);
	const resolvedResourceValue = await scope.resolve(resourceValue);

	console.log("double", resolvedDouble.get()); // 2
	console.log("triple", resolvedTriple.get()); // 3

	console.log(">>", "update value to 2");
	scope.update(value, 2);
	console.log("double", resolvedDouble.get()); // 2

	await scope.once(value);

	console.log(resolvedDouble.get()); // 4
	console.log(resolvedTriple.get()); // 6

	await scope.update(value, 3);
	console.log(resolvedDouble.get()); // 4
	console.log(resolvedTriple.get()); // 9

	scope.dispose();
}

main();

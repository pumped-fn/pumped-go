import { derive, mutable, provide, ref } from "@pumped-fn/core"

type Todo = {
  id: string
  content: string
  completed: boolean
}

const idGenerator = provide(() => {
  let id = 0
  return () => {
    id++
    return id.toString()
  }
})

const todos = provide(() => mutable([] as Todo[]))

const selectedTodoId = provide(() => mutable(null as string | null))

const setSelectedTodoId = derive(
  [ref(selectedTodoId)],
  ([ref], scope) => {
    return (id: string | null) => {
      scope.update(ref.get(), () => id)
    }
  })

const selectedTodo = derive([selectedTodoId, todos], ([selectedTodoId, todos]) => {
  const todo = selectedTodoId.get() ? todos.get().find((todo) => todo.id === selectedTodoId.get()) : null

  console.log('selectedTodo', todo)
  return todo
})

const todosController = derive([idGenerator, ref(todos)], ([idGenerator, refTodos], scope) => {
  return {
    addTodo: (todo: Omit<Todo, 'id'>) => {
      scope.update(refTodos.get(), (v) => [...v, { ...todo, id: idGenerator.get()() }])
    },
    removeTodo: (id: string) => {
      scope.update(refTodos.get(), (v) => v.filter((todo) => todo.id !== id))
    },
    toggleComplete: (id: string) => {
      scope.update(refTodos.get(), (v) => v.map((todo) => {
        if (todo.id === id) {
          return { ...todo, completed: !todo.completed }
        }
        return todo
      }))
    }
  }
})

export const todoApp = {
  todos,
  todosController,
  selectedTodo,
  setSelectedTodoId
}
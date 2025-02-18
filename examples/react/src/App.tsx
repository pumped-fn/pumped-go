import React, { Suspense, useState } from 'react';
import { ScopeProvider, useResolve } from '@pumped-fn/react';

import { counterApp } from './counter.pumped';
import { todoApp } from './todo.pumped';

function App() {
  const counter = useResolve(counterApp.counter)
  useResolve(counterApp.timer)

  return (
    <>
      <h1>
        Counter: {counter}
      </h1>
    </>
  )
}

function ChangeConfig() {
  const controller = useResolve(counterApp.configController)
  const config = useResolve(counterApp.config)

  return <>
    <input type="number" value={config.increment} onChange={(e) => controller.changeIncrement(+e.target.value)} />
    <input type="number" value={config.interval} onChange={(e) => controller.changeInterval(+e.target.value)} />
  </>
}

function TodoList() {
  const todos = useResolve(todoApp.todos)
  const setSelectedTodoId = useResolve(todoApp.setSelectedTodoId)
  console.log(todos)
  return <>
    <h1>Todo list</h1>
    {todos.map(todo => <div key={todo.id} onClick={() => setSelectedTodoId(todo.id)}>{todo.content}</div>)}
  </>
}

function TodoDetail() {
  const todo = useResolve(todoApp.selectedTodo)
  const setSelectedTodoId = useResolve(todoApp.setSelectedTodoId)
  

  if (!todo) return null

  return <>
    <h1>{todo.content}</h1>
    <button onClick={() => setSelectedTodoId(null)}>Close</button>
  </>
  return <>{JSON.stringify(todo)}</>
}

function TodoForm() {
  const controller = useResolve(todoApp.todosController)

  return <>
    <form onSubmit={(e) => {
      e.preventDefault()
      const content = e.currentTarget.content.value
      controller.addTodo({ content, completed: false })
      e.currentTarget.reset()
    }}>
      <input type="text" name="content" />
      <button type="submit">Add todo</button>
    </form>
  </>
}

export default function AppWrapper() {
  return (
    <ScopeProvider>
      {/* <Suspense>
        <App />
      </Suspense>
      <Suspense>
        <ChangeConfig />
      </Suspense> */}
      <Suspense>
        <TodoDetail />
      </Suspense>
      <Suspense>
        <TodoList />
        <TodoForm />
      </Suspense>
    </ScopeProvider>
  );
}

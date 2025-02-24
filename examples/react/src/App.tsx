import { Suspense, useMemo } from "react";
import { ScopeProvider, useResolve, useResolveMany } from "@pumped-fn/react";

import { todoApp } from "./todo.pumped";

let rerender = 0
function TodoList() {
  const [
    todos,
    setSeletectedTodoId,
    controller
  ] = useResolveMany(todoApp.todos, todoApp.setSelectedTodoId, todoApp.todosController);

  ++rerender
  console.log(rerender)

  return (
    <>
      <h1>Todo list</h1>
      {todos.map((todo) => (
        <div key={todo.id} onClick={() => setSeletectedTodoId(todo.id)}>
          {todo.content}
          - 
          <button onClick={() => controller.removeTodo(todo.id)}>Remove</button>
        </div>
      ))}
    </>
  );
}

function TodoDetail() {
  const [todo, setSelectedTodoId, controller] = useResolveMany(
    todoApp.selectedTodo,
    todoApp.setSelectedTodoId,
    todoApp.todosController
  );

  if (!todo) return null;

  return (
    <>
      <h1>{todo.content}</h1>
      <label>
        Mark as completed:
        <input type="checkbox" checked={todo.completed} onChange={() => controller.toggleComplete(todo.id)} />
      </label>
      <button onClick={() => setSelectedTodoId(null)}>Close</button>
    </>
  );
}

function TodoForm() {
  const controller = useResolve(todoApp.todosController);

  return (
    <>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const content = e.currentTarget.content.value;
          controller.addTodo({ content, completed: false });
          e.currentTarget.reset();
        }}
      >
        <input type="text" name="content" />
        <button type="submit">Add todo</button>
      </form>
    </>
  );
}

function CompletedTodoList() {
  const todos = useResolve(
    todoApp.todos, 
    useMemo(() => (todos) => todos.filter((todo) => todo.completed), [])
  );

  return (
    <>
    <h1>Completed todos {todos.length}</h1>
      {todos.map((todo) => (
        <div key={todo.id}>{todo.content}</div>
      ))}
    </>
  );
}

export default function AppWrapper() {
  return (
    <ScopeProvider>
      <Suspense>
        <TodoDetail />
        <TodoList />
        <TodoForm />
        <CompletedTodoList />
      </Suspense>
    </ScopeProvider>
  );
}

import { derive, mutable, provide, ref } from "@pumped-fn/core";

export type Todo = {
  id: string;
  content: string;
  completed: boolean;
};

const idGenerator = provide(() => {
  let id = 0;
  return () => {
    id++;
    return id.toString();
  };
});

const todos = provide(() => mutable([] as Todo[]));

const selectedTodoId = provide(() => mutable(null as string | null));

const setSelectedTodoId = derive([ref(selectedTodoId)], ([ref], scope) => {
  return (id: string | null) => {
    scope.update(ref, () => id);
  };
});

const selectedTodo = derive([selectedTodoId, todos], ([selectedTodoId, todos]) => {
  const todo = selectedTodoId ? todos.find((todo) => todo.id === selectedTodoId) : null;
  return todo;
});

const todosController = derive([idGenerator, ref(todos)], ([idGenerator, refTodos], scope) => {
  return {
    addTodo: (todo: Omit<Todo, "id">) => {
      scope.update(refTodos, (v) => [...v, { ...todo, id: idGenerator() }]);
    },
    removeTodo: (id: string) => {
      scope.update(refTodos, (v) => v.filter((todo) => todo.id !== id));
    },
    toggleComplete: (id: string) => {
      scope.update(refTodos, (v) =>
        v.map((todo) => {
          if (todo.id === id) {
            return { ...todo, completed: !todo.completed };
          }
          return todo;
        }),
      );
    },
  };
});

export const todoApp = {
  todos,
  todosController,
  selectedTodo,
  setSelectedTodoId,
};

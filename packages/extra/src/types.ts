import { Executor } from "@pumped-fn/core";

export interface Endpoint<Def, Req, Res> {
  readonly definition: Def;
  makeRequest(...args: unknown[]): Req;
  makeResponse(...args: unknown[]): Res;
}

type AnyEndpoint = Endpoint<unknown, unknown, unknown>;

export interface ClientImplentation<T extends AnyEndpoint, ClientShape> {
  (endpoint: T): ClientShape;
}

export declare function createClient<E extends AnyEndpoint, ClientShape>(
  endpoint: E,
  implementation: ClientImplentation<E, ClientShape> | Executor<ClientImplentation<E, ClientShape>>,
): Executor<ClientShape>;

interface ServerImplementation<E extends AnyEndpoint> {
  (endpoint: E, implementation: Implementation<E>): void;
}

interface Implementation<E extends AnyEndpoint> {
  (request: ReturnType<E["makeRequest"]>, def: E["definition"]): Promise<ReturnType<E["makeResponse"]>>;
}

export declare function createServerImplementation<E extends AnyEndpoint>(
  endpoint: E,
  serverImplementation: Executor<ServerImplementation<E>> | ServerImplementation<E>,
  implementation: Executor<Implementation<E>> | Implementation<E>,
): Executor<void>;

type HttpDefinition = {
  method: "GET" | "POST" | "PUT" | "DELETE";
  path: string;
};

type HttpRequest<Body = unknown> = {
  headers: Record<string, string | string[]>;
  body: Body;
};

type HttpResponse<Body = unknown> = {
  status: number;
  headers: Record<string, string | string[]>;
  body: Body;
};

interface HttpEndpoint extends Endpoint<HttpDefinition, HttpRequest, HttpResponse> {}

interface HttpClient extends ClientImplentation<HttpEndpoint, (req: HttpRequest) => HttpResponse> {}

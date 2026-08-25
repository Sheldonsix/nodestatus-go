export interface IResp<T = any> {
  code: 0 | 1;
  data: T;
  msg: string;
}

export interface IServer {
  id: number;
  username: string;
  name: string;
  type: string;
  location: string;
  region: string;
  disabled: boolean;
  order: number;
}

export interface ServerItem extends Omit<IServer, 'disabled'> {
  last_active?: number;
  status: Record<string, any>;
}

export interface Event {
  id: number;
  username: string;
  resolved: boolean;
  created_at: string | number;
  updated_at: string | number;
}

export interface IWebConfig {
  title: string;
  subTitle: string;
  headTitle: string;
}

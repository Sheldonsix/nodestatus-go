import type {
  FormInstance,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type {
  FC,
  ReactElement,
  Reducer,
} from 'react';
import type { KeyedMutator } from 'swr';
import type { IResp, IServer } from '../types';
import {
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  MenuOutlined,
} from '@ant-design/icons';
import {
  AutoComplete,
  Button,
  Form,
  Input,
  Modal,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { arrayMoveImmutable } from 'array-move';
import countries from 'i18n-iso-countries';
import i18nEn from 'i18n-iso-countries/langs/en.json';
import i18nZh from 'i18n-iso-countries/langs/zh.json';
import React, {
  useCallback,
  useMemo,
  useReducer,
  useState,
} from 'react';
import { DragDropContext, Draggable, Droppable } from 'react-beautiful-dnd';
import useSWR from 'swr';
import Loading from '../components/Loading';
import { useI18n } from '../i18n';
import api from '../lib/api';
import { notify } from '../utils';

const { Title } = Typography;
countries.registerLocale(i18nZh);
countries.registerLocale(i18nEn);

interface ActionType {
  type: 'showModal' | 'showImportForm' | 'reverseSortEnabled' | 'resetState' | 'setInstallationScript' | 'setNode';
  payload?: {
    form?: FormInstance;
    mutate?: KeyedMutator<any>;
    installationScript?: string;
    currentNode?: string;
  };
}

const initialState = {
  currentNode: '',
  installationScript: '',
  showModal: false,
  sortEnabled: false,
  isImport: false,
};

const reducer: Reducer<typeof initialState, ActionType> = (state, action) => {
  const {
    mutate,
    form,
    installationScript = '',
    currentNode = '',
  } = action.payload ?? {};
  switch (action.type) {
    case 'showModal':
      return {
        ...state,
        showModal: true,
      };
    case 'reverseSortEnabled':
      return { ...state, sortEnabled: !state.sortEnabled };
    case 'setInstallationScript':
      return { ...state, installationScript };
    case 'showImportForm': {
      return { ...state, showModal: true, isImport: true };
    }
    case 'setNode':
      return {
        ...state,
        showModal: true,
        currentNode,
        installationScript,
      };
    case 'resetState':
      mutate?.();
      form?.resetFields();
      return {
        ...state,
        currentNode: '',
        installationScript: '',
        showModal: false,
        isImport: false,
      };
    default:
      throw new Error();
  }
};

function basicValidator(_: unknown, value: string) {
  return [' ', '+', '&', '%', '/', '\\', '?', '#']
    .some(v => value.includes(v))
    ? Promise.reject(new Error('不能包含空格和特殊字符'))
    : Promise.resolve();
}

function parseInstallationScript(username: string, password: string): string {
  const protocol = document.location.protocol.replace('http', 'ws');
  const { host } = window.location;
  const dsn = `${protocol}//${username || 'USERNAME_YOU_SET'}:${password || 'PASSWORD_YOU_SET'}@${host}`;
  const quotedDsn = `'${dsn.replace(/'/g, '\'\\\'\'')}'`;
  return `wget -O /tmp/nodestatus-client-install.sh https://raw.githubusercontent.com/Sheldonsix/nodestatus/master/scripts/install-client-go.sh && sh /tmp/nodestatus-client-install.sh --dsn ${quotedDsn}`;
}

const Management: FC = () => {
  const [regionResult, setRegionResult] = useState<string[]>([]);
  const [state, dispatch] = useReducer(reducer, initialState);
  const { data, mutate } = useSWR<IResp<IServer[]>>('/api/admin/servers');

  const [form] = Form.useForm<IServer & { password: string }>();
  const { confirm } = Modal;
  const dataSource = data?.data;

  const { t } = useI18n();

  const handleModify = useCallback(() => {
    const data = form.getFieldsValue();
    api.put('/api/admin/servers', { json: { username: state.currentNode, data } }).json<IResp>().then((res) => {
      notify('成功', res.msg, 'success');
      dispatch({ type: 'resetState', payload: { form, mutate } });
    });
  }, [state.currentNode, form, mutate]);

  const handleCreate = useCallback(() => {
    const data = form.getFieldsValue();
    api.post('/api/admin/servers', { json: { ...data } }).json<IResp>().then((res) => {
      notify('成功', res.msg, 'success');
      dispatch({ type: 'resetState', payload: { form, mutate } });
    });
  }, [form, mutate]);

  const handleDelete = useCallback((username: string) => {
    api.delete(`/api/admin/servers/${username}`).json<IResp>().then((res) => {
      notify('成功', res.msg, 'success');
      dispatch({ type: 'resetState', payload: { form, mutate } });
    });
  }, [form, mutate]);

  const handleSortOrder = useCallback((order: number[]) => {
    api.put('/api/admin/servers/order', { json: { order } }).json<IResp>().then((res) => {
      notify('成功', res.msg, 'success');
      dispatch({ type: 'resetState', payload: { form, mutate } });
    });
  }, [form, mutate]);

  const columns = useMemo<ColumnsType<IServer>>(() => ([
    {
      title: '排序',
      dataIndex: 'sort',
      width: 30,
      align: 'center',
      render: () => undefined,
    },
    {
      title: '服务器',
      dataIndex: 'server',
      align: 'left',
      render(_, record) {
        return (
          <div className="flex items-center  text-sm">
            <svg viewBox="0 0 100 100" className="mr-3 block h-12 w-12">
              <use xlinkHref={`#${record.region}`} />
            </svg>
            <div className="whitespace-nowrap">
              <p className="font-semibold">{record.name}</p>
              <p className="text-left text-xs text-gray-600">{record.location}</p>
            </div>
          </div>
        );
      },
    },
    {
      title: '用户名',
      dataIndex: 'username',
      align: 'center',
    },
    {
      title: '类型',
      dataIndex: 'type',
      align: 'center',
    },
    {
      title: '位置',
      dataIndex: 'location',
      align: 'center',
    },
    {
      title: '国家/地区',
      dataIndex: 'region',
      align: 'center',
    },
    {
      title: '状态',
      dataIndex: 'disabled',
      align: 'center',
      render: disabled => (
        disabled
          ? <Tag color="error">已禁用</Tag>
          : <Tag color="success">已启用</Tag>
      ),
    },
    {
      title: '操作',
      dataIndex: 'action',
      align: 'center',
      render(_, record) {
        return (
          <div className="flex justify-evenly items-center">
            <EditOutlined onClick={() => {
              form.setFieldsValue(record);
              dispatch({
                type: 'setNode',
                payload: {
                  currentNode: record.username,
                  installationScript: parseInstallationScript(record.username, ''),
                },
              });
            }}
            />
            <DeleteOutlined onClick={() => confirm({
              title: '确定要删除这个节点吗？',
              icon: <ExclamationCircleOutlined />,
              onOk: () => handleDelete(record.username),
            })}
            />
          </div>
        );
      },
    },
  ] satisfies ColumnsType<IServer>).filter(item => item.dataIndex !== 'sort' || state.sortEnabled), [state.sortEnabled, confirm, form, handleDelete]);

  const TableFooter = useCallback(() => (
    <>
      <Button type="primary" className="mr-6" onClick={() => dispatch({ type: 'showModal' })}>新建</Button>
      <Button
        type="primary"
        className="mr-6"
        onClick={() => dispatch({ type: 'showImportForm' })}
      >
        导入
      </Button>
      <Button
        type="primary"
        danger={state.sortEnabled}
        onClick={() => {
          if (state.sortEnabled) {
            const order = dataSource.map(item => item.id);
            order.reverse();
            handleSortOrder(order);
          }
          dispatch({ type: 'reverseSortEnabled' });
        }}
      >
        {!state.sortEnabled ? '排序' : '保存'}
      </Button>
    </>
  ), [dataSource, handleSortOrder, state.sortEnabled]);

  const DraggableContainer = useCallback<FC>(props => (
    <Droppable droppableId="table">
      {
        provided => (
          <tbody {...props} {...provided.droppableProps} ref={provided.innerRef}>
            {props.children}
            {provided.placeholder}
          </tbody>
        )
      }
    </Droppable>
  ), []);

  const DraggableBodyRow = useCallback<FC<any>>((props) => {
    const index = dataSource.findIndex(x => x.id === props['data-row-key']);
    return (
      <Draggable
        draggableId={props['data-row-key']?.toString()}
        index={index}
        isDragDisabled={!state.sortEnabled}
      >
        {(provided) => {
          const children = props.children?.map?.((el: ReactElement) => {
            if (el.props.dataIndex === 'sort') {
              const props = el.props ? { ...el.props } : {};
              props.render = () => (
                <MenuOutlined
                  style={{ cursor: 'grab', color: '#999' }}
                  {...provided.dragHandleProps}
                />
              );
              return React.cloneElement(el, props);
            }
            return el;
          }) || props.children;
          return (
            <tr {...props} {...provided.draggableProps} ref={provided.innerRef}>
              {children}
            </tr>
          );
        }}
      </Draggable>
    );
  }, [dataSource, state.sortEnabled]);

  return (
    <>
      <Title level={2} className="my-6">{t('management')}</Title>
      {
        data
          ? (
              <DragDropContext
                onDragEnd={(result) => {
                  const { destination, source } = result;
                  if (!destination)
                    return;
                  if (destination.droppableId === source.droppableId && destination.index === source.index)
                    return;
                  const newDataSource = arrayMoveImmutable(dataSource, source.index, destination.index);
                  mutate({ ...data, data: newDataSource }, false).then();
                }}
              >
                <Table
                  dataSource={dataSource}
                  columns={columns}
                  rowKey="id"
                  components={{
                    body: {
                      wrapper: DraggableContainer,
                      row: DraggableBodyRow,
                    },
                  }}
                  pagination={state.sortEnabled ? false : undefined}
                  footer={TableFooter}
                />
                <Modal
                  title={state.currentNode ? '修改配置' : '新建节点'}
                  visible={state.showModal}
                  onOk={state.currentNode ? handleModify : handleCreate}
                  onCancel={() => dispatch({ type: 'resetState', payload: { form } })}
                  className="top-12"
                >
                  <Form
                    layout="vertical"
                    form={form}
                    onValuesChange={(field, allFields) => {
                      if (field.username || field.password) {
                        dispatch({
                          type: 'setInstallationScript',
                          payload: {
                            installationScript: parseInstallationScript(
                              field.username || allFields.username,
                              field.password || allFields.password,
                            ),
                          },
                        });
                      }
                    }}
                  >
                    {state.isImport
                      ? (
                          <Form.Item label="数据" name="data">
                            <Input.TextArea rows={4} />
                          </Form.Item>
                        )
                      : (
                          <>
                            <Form.Item
                              label="用户名"
                              name="username"
                              rules={
                                [
                                  {
                                    validator: basicValidator,
                                  },
                                ]
                              }
                            >
                              <Input />
                            </Form.Item>
                            <Form.Item
                              label="密码"
                              name="password"
                              rules={
                                [
                                  {
                                    validator: basicValidator,
                                  },
                                ]
                              }
                            >
                              <Input.Password placeholder="留空不修改" />
                            </Form.Item>
                            <Form.Item label="名称" name="name">
                              <Input />
                            </Form.Item>
                            <Form.Item label="类型" name="type">
                              <Input />
                            </Form.Item>
                            <Form.Item label="位置" name="location">
                              <Input />
                            </Form.Item>
                            <Form.Item
                              label="国家/地区"
                              name="region"
                              rules={[{
                                validator(_, value) {
                                  if (countries.isValid(value))
                                    return Promise.resolve();
                                  return Promise.reject(new Error('未找到国家或地区'));
                                },
                              }]}
                            >
                              <AutoComplete
                                options={regionResult.map(value => ({
                                  value,
                                  label: value,
                                }))}
                                onChange={(value: unknown) => {
                                  if (typeof value !== 'string')
                                    return [];
                                  const code = countries.getAlpha2Code(value, 'zh');
                                  const codeEn = countries.getAlpha2Code(value, 'en');
                                  const fullMatch = [code, codeEn].filter(v => !!v);
                                  return fullMatch.length
                                    ? setRegionResult(fullMatch)
                                    : setRegionResult(
                                        Object.keys(countries.getAlpha2Codes()).filter(v => v.startsWith(value.toUpperCase())),
                                      );
                                }}
                              >
                                <Input />
                              </AutoComplete>
                            </Form.Item>
                            <Form.Item label="禁用" name="disabled" valuePropName="checked">
                              <Switch />
                            </Form.Item>
                            <Form.Item label="安装脚本">
                              <code
                                className="bg-gray-200 px-2 py-0.5 leading-6 rounded break-all"
                              >
                                {state.installationScript}
                              </code>
                            </Form.Item>
                          </>
                        )}
                  </Form>
                </Modal>
              </DragDropContext>
            )
          : <Loading />
      }
    </>
  );
};

export default Management;

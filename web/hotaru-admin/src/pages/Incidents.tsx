import type { ColumnsType } from 'antd/es/table';
import type {
  FC,
} from 'react';
import type { Event as IEvent, IResp } from '../types';
import {
  ExclamationCircleOutlined,
} from '@ant-design/icons';
import {
  Button,
  Modal,
  Table,
  Tag,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import React, {
  useCallback,
  useMemo,
  useState,
} from 'react';
import useSWR from 'swr';
import Loading from '../components/Loading';
import api from '../lib/api';
import { notify } from '../utils';

const { Title } = Typography;

const Incidents: FC = () => {
  const [currentPage, setCurrentPage] = useState(1);
  // this may not be the optimum solution for using SWR in React
  const {
    data: resp,
    mutate,
  } = useSWR<IResp<{ count: number; list: IEvent[] }>>(`/api/admin/events?size=10&offset=${(currentPage - 1) * 10}`);
  const { count, list: dataList } = resp?.data || {};

  const handleDeleteEvent = useCallback((id: number) => {
    api.delete(`/api/admin/events/${id}`).json<IResp>().then((res) => {
      notify('成功', res.msg, 'success');
      return mutate();
    });
  }, [mutate]);

  const columns: ColumnsType<IEvent> = useMemo(() => [
    {
      title: '服务器',
      dataIndex: 'server',
      render(_, record) {
        return record.username;
      },
    },
    {
      title: '类型',
      dataIndex: 'type',
      render() {
        return <Tag color="error">宕机</Tag>;
      },
    },
    {
      title: '恢复状态',
      dataIndex: 'resolved',
      render(resolved) {
        return resolved
          ? <Tag color="success">已恢复</Tag>
          : <Tag color="error">未恢复</Tag>;
      },
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      render(createdAt) {
        return dayjs(createdAt).format('YYYY-MM-DD hh:mm');
      },
    },
    {
      title: '恢复时间',
      dataIndex: 'updated_at',
      render(updatedAt, record) {
        return record.resolved ? dayjs(updatedAt).format('YYYY-MM-DD hh:mm') : '';
      },
    },
    {
      title: '操作',
      dataIndex: 'action',
      align: 'center',
      render(_, record) {
        return (
          <Button
            danger
            onClick={() => Modal.confirm({
              title: '确定要删除这条记录吗？',
              icon: <ExclamationCircleOutlined />,
              onOk: () => handleDeleteEvent(record.id),
            })}
          >
            删除
          </Button>
        );
      },
    },
  ], [handleDeleteEvent]);

  const Footer = useCallback(() => (
    <div>
      <Button
        type="primary"
        danger
        onClick={() => Modal.confirm({
          title: '确定要删除全部记录吗？',
          icon: <ExclamationCircleOutlined />,
          onOk: () => api.delete('/api/admin/events').json<IResp>().then((res) => {
            notify('成功', res.msg, 'success');
            return mutate();
          }),
        })}
      >
        全部删除
      </Button>
    </div>
  ), [mutate]);

  return (
    <>
      <Title level={2} className="my-6 text-3xl">故障记录</Title>
      {
        dataList
          ? (
              <Table
                className="rounded-lg max-w-full"
                dataSource={dataList}
                columns={columns}
                footer={Footer}
                rowKey="id"
                pagination={{
                  total: count,
                  current: currentPage,
                  onChange: page => setCurrentPage(page),
                }}
              />
            )
          : <Loading />
      }
    </>
  );
};

export default Incidents;

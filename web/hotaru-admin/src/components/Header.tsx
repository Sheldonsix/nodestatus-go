import type { FC } from 'react';
import {
  GlobalOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Dropdown, Menu } from 'antd';
import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useI18n } from '../i18n';

interface Props {
  collapsed: {
    isCollapsed: boolean;
    toggleCollapsed: () => void;
  };
}

const Header: FC<Props> = (props) => {
  const navigate = useNavigate();
  const { isCollapsed, toggleCollapsed } = props.collapsed;
  const { t, setLang } = useI18n();

  const menu = (
    <Menu
      items={[
        {
          key: 'logout',
          label: t('logout'),
          icon: <LogoutOutlined className="mr-2 align-middle" />,
          className: 'align-middle',
        },
      ]}
      onClick={({ key }) => {
        if (key === 'logout') {
          localStorage.removeItem('token');
          navigate('/login');
        }
      }}
    />
  );

  const globalMenu = (
    <Menu
      items={[
        {
          key: 'en',
          label: 'English',
          className: 'align-middle',
        },
        {
          key: 'zh',
          label: '简体中文',
          className: 'align-middle',
        },
      ]}
      onClick={({ key }) => {
        if (key === 'en' || key === 'zh') {
          setLang(key);
        }
      }}
    />
  );

  return (
    <div className="h-full flex items-center justify-between">
      {React.createElement(isCollapsed ? MenuUnfoldOutlined : MenuFoldOutlined, {
        className: 'text-2xl',
        onClick: toggleCollapsed,
      })}
      <div className="flex items-center gap-2">

        <Dropdown overlay={globalMenu} placement="bottom">
          <Avatar size={40} icon={<GlobalOutlined />} />
        </Dropdown>
        <Dropdown overlay={menu} placement="bottom">
          <Avatar size={40} icon={<UserOutlined />} />
        </Dropdown>
      </div>
    </div>
  );
};

export default Header;

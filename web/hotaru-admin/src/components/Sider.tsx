import type { FC } from 'react';
import {
  AlertFilled,
  DashboardOutlined,
  ProfileFilled,
} from '@ant-design/icons';
import { Menu } from 'antd';
import React from 'react';
import { useNavigate } from 'react-router-dom';
import logo from '../assets/img/logo.png';
import smallLogo from '../assets/img/logo.svg';
import { useI18n } from '../i18n';

interface Props {
  isCollapsed: boolean;
}

const Sider: FC<Props> = ({ isCollapsed }) => {
  const navigate = useNavigate();
  const { t } = useI18n();

  const menus = [
    {
      label: t('dashboard'),
      icon: <DashboardOutlined />,
      key: '/dashboard',
    },
    {
      label: t('management'),
      icon: <ProfileFilled />,
      key: '/management',
    },
    {
      label: t('incidents'),
      icon: <AlertFilled />,
      key: '/incidents',
    },
  ].map(menu => ({
    ...menu,
    className: 'h-12',
    style: { lineHeight: '3rem' },
  }));
  return (
    <>
      <img src={logo} alt="" className="m-auto p-4 lg:hidden" draggable="false" />
      <img
        src={isCollapsed ? smallLogo : logo}
        alt=""
        className="hidden lg:inline-block  m-auto p-4"
        draggable="false"
      />
      <Menu
        theme="dark"
        mode="inline"
        items={menus}
        onClick={({ key }) => navigate(key)}
      />
    </>
  );
};

export default Sider;

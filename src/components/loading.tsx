import React from "react";
import { LoadingOutlined } from "@ant-design/icons";
import { Spin, theme } from "antd";

const Loading: React.FC = () => {
  const {
    token: { colorBgContainer },
  } = theme.useToken();

  return (
    <div
      style={{
        width: "100%",
        height: "100vh",
        backgroundColor: colorBgContainer,
        display: "flex",
        justifyContent: "center",
        alignItems: "center",
      }}
    >
      <Spin indicator={<LoadingOutlined style={{ fontSize: 48 }} spin />} />
    </div>
  );
};

export default Loading;

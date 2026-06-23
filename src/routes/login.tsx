import { useState } from "react";
import { useNavigate } from "react-router";
import { Alert, Button, Card, Form, Input, Space, Typography } from "antd";
import { LockOutlined, UserOutlined } from "@ant-design/icons";
import { useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";

import { Login } from "src/api";
import { currentUserAtom } from "src/atoms/auth";

const { Title, Text } = Typography;

export default function LoginPage() {
  useLingui();
  const navigate = useNavigate();
  const setCurrentUser = useSetAtom(currentUserAtom);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onFinish = async (values: { email: string; password: string }) => {
    setError(null);
    setLoading(true);
    try {
      const { user } = await Login(values.email, values.password);
      setCurrentUser(user);
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : t`Login failed`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#f0f2f5",
      }}
    >
      <Card style={{ width: 380, boxShadow: "0 4px 24px rgba(0,0,0,0.08)" }}>
        <Space direction="vertical" size="large" style={{ width: "100%", textAlign: "center" }}>
          <div>
            <img src="/logo-light.png" alt="FaturaCloud" style={{ height: 48, marginBottom: 8, filter: "invert(1) brightness(0)" }} />
            <Title level={4} style={{ marginBottom: 4, marginTop: 0 }}>
              FaturaCloud
            </Title>
            <Text type="secondary">
              <Trans>Sign in to your account</Trans>
            </Text>
          </div>

          {error && <Alert type="error" message={error} showIcon />}

          <Form layout="vertical" onFinish={onFinish} requiredMark={false} style={{ textAlign: "left" }}>
            <Form.Item
              name="email"
              rules={[{ required: true, type: "email", message: t`Please enter a valid email` }]}
            >
              <Input prefix={<UserOutlined />} placeholder="Email" size="large" />
            </Form.Item>
            <Form.Item
              name="password"
              rules={[{ required: true, message: t`Please enter your password` }]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder={t`Password`} size="large" />
            </Form.Item>
            <Form.Item style={{ marginBottom: 0 }}>
              <Button type="primary" htmlType="submit" block size="large" loading={loading}>
                <Trans>Sign in</Trans>
              </Button>
            </Form.Item>
          </Form>
        </Space>
      </Card>
    </div>
  );
}

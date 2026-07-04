import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { Alert, Button, Card, Divider, Form, Input, Space, Typography, theme } from "antd";
import { LockOutlined, UserOutlined } from "@ant-design/icons";
import { useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";

import { Login, GetOidcEnabled } from "src/api";
import { currentUserAtom } from "src/atoms/auth";
import Wordmark from "src/components/wordmark";

const { Text } = Typography;

export default function LoginPage() {
  useLingui();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const setCurrentUser = useSetAtom(currentUserAtom);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [ssoEnabled, setSsoEnabled] = useState(false);
  const {
    token: { colorBgLayout },
  } = theme.useToken();

  useEffect(() => {
    GetOidcEnabled().then(setSsoEnabled);
  }, []);

  useEffect(() => {
    if (searchParams.get("error") === "sso_failed") {
      setError(t`Single sign-on login failed — try again, or use your email and password below.`);
    }
  }, [searchParams]);

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
        background: colorBgLayout,
      }}
    >
      <Card style={{ width: 380, boxShadow: "0 4px 24px rgba(0,0,0,0.08)" }}>
        <Space direction="vertical" size="large" style={{ width: "100%", textAlign: "center" }}>
          <div>
            <img src="/logo-minimal.png" alt="FaturaCloud" style={{ height: 72, marginBottom: 14 }} />
            <div style={{ marginBottom: 6 }}>
              <Wordmark fontSize={26} />
            </div>
            <Text type="secondary">
              <Trans>Sign in to your account</Trans>
            </Text>
          </div>

          {error && <Alert type="error" message={error} showIcon />}

          {ssoEnabled && (
            <>
              <Button
                block
                size="large"
                onClick={() => {
                  // Full browser navigation, not a fetch — this must be a
                  // real top-level redirect for the OAuth dance to work.
                  window.location.href = "/api/auth/oidc/login";
                }}
              >
                <Trans>Sign in with SSO</Trans>
              </Button>
              <Divider style={{ margin: 0 }}>
                <Text type="secondary" style={{ fontWeight: "normal" }}>
                  <Trans>or</Trans>
                </Text>
              </Divider>
            </>
          )}

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

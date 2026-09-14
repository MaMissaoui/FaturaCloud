import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Checkbox, Drawer, Form, Input, Popconfirm, Space } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";

import {
  unitOfMeasureIdAtom,
  unitOfMeasureAtom,
  unitsOfMeasureAtom,
  deleteUnitOfMeasureAtom,
} from "src/atoms/unit-of-measure";

const UnitOfMeasureForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [unitOfMeasureId, setUnitOfMeasureId] = useAtom(unitOfMeasureIdAtom);
  const unitsOfMeasure = useAtomValue(unitsOfMeasureAtom);
  const setUnitOfMeasure = useSetAtom(unitOfMeasureAtom);
  const deleteUnitOfMeasure = useSetAtom(deleteUnitOfMeasureAtom);
  const [submitting, setSubmitting] = useState(false);

  const isVisible = get(location.state, "unitOfMeasureModal", false);

  const unitOfMeasure = useMemo(() => {
    if (!unitOfMeasureId) return null;
    return unitsOfMeasure.find((u: any) => u.id === unitOfMeasureId) ?? null;
  }, [unitsOfMeasure, unitOfMeasureId]);

  const handleClose = () => {
    setUnitOfMeasureId(null);
    form.resetFields();
    navigate(location.pathname, { state: { unitOfMeasureModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setUnitOfMeasure(values);
      handleClose();
    } catch {
      // setUnitOfMeasure already toasted the error — keep the drawer open
      // with the user's input intact rather than closing on a failed save.
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (unitOfMeasureId) {
      setSubmitting(true);
      const deleted = await deleteUnitOfMeasure(unitOfMeasureId);
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navId = get(location.state, "unitOfMeasureId");
    if (isVisible && navId) {
      setUnitOfMeasureId(navId);
    } else if (!isVisible) {
      setUnitOfMeasureId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setUnitOfMeasureId, form]);

  useEffect(() => {
    if (unitOfMeasure) {
      form.setFieldsValue({ ...unitOfMeasure, isDefault: !!unitOfMeasure.isDefault });
    } else if (!unitOfMeasureId) {
      form.resetFields();
    }
  }, [unitOfMeasure, unitOfMeasureId, form]);

  return (
    <Drawer
      title={
        unitOfMeasureId ? <Trans>Edit unit of measure</Trans> : <Trans>New unit of measure</Trans>
      }
      open={isVisible}
      placement="right"
      size={420}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {unitOfMeasureId && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this unit of measure?</Trans>}
                onConfirm={handleDelete}
                okText={<Trans>Yes</Trans>}
                cancelText={<Trans>No</Trans>}
                placement="topRight"
              >
                <Button danger icon={<DeleteOutlined />} loading={submitting}>
                  <Trans>Delete</Trans>
                </Button>
              </Popconfirm>
            )}
          </div>
          <Space>
            <Button onClick={handleClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        </div>
      }
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Form.Item
          name="name"
          label={<Trans>Name</Trans>}
          rules={[{ required: true, message: t`This field is required!` }]}
        >
          <Input placeholder={t`e.g. kg, piece, hour`} />
        </Form.Item>
        <Form.Item name="isDefault" valuePropName="checked">
          <Checkbox>
            <Trans>Default for new products</Trans>
          </Checkbox>
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default UnitOfMeasureForm;

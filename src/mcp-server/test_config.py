import importlib
import os
import unittest


class ConfigTest(unittest.TestCase):
    def test_reads_pingo_environment_variables(self):
        old_agent_id = os.environ.get("PINGO_AGENT_ID")
        old_sidecar_url = os.environ.get("PINGO_SIDECAR_URL")
        os.environ["PINGO_AGENT_ID"] = "agent-b"
        os.environ["PINGO_SIDECAR_URL"] = "http://127.0.0.1:19999"
        try:
            config = importlib.reload(importlib.import_module("config"))
            self.assertEqual(config.AGENT_ID, "agent-b")
            self.assertEqual(config.SIDECAR_URL, "http://127.0.0.1:19999")
        finally:
            if old_agent_id is None:
                os.environ.pop("PINGO_AGENT_ID", None)
            else:
                os.environ["PINGO_AGENT_ID"] = old_agent_id
            if old_sidecar_url is None:
                os.environ.pop("PINGO_SIDECAR_URL", None)
            else:
                os.environ["PINGO_SIDECAR_URL"] = old_sidecar_url


if __name__ == "__main__":
    unittest.main()

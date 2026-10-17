package com.overdrive.app.server;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.json.JSONObject;
import org.junit.Test;

/** GET /api/performance/soh/nominal while the daemon is still starting (e.g. right after an app update). */
public class SohNominalResponseTest {

    @Test
    public void estimatorNotAttachedAnswersUserNominalFromConfig() throws Exception {
        JSONObject response = PerformanceApiHandler.nominalResponse(null, () -> 82.5);
        assertEquals(82.5, response.getDouble("nominalKwh"), 0.0);
        assertEquals("user", response.getString("nominalSource"));
    }

    @Test
    public void estimatorNotAttachedWithoutUserNominalIsNotReady() throws Exception {
        assertNull(PerformanceApiHandler.nominalResponse(null, () -> 0.0));
    }

    @Test
    public void unreadableConfigIsNotReady() throws Exception {
        assertNull(PerformanceApiHandler.nominalResponse(null, () -> {
            throw new IllegalStateException("config lock unavailable");
        }));
    }
}

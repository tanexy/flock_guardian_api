#include <Arduino.h>
#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <PubSubClient.h>
#include <HTTPClient.h>
#include <ArduinoJson.h>
#include "DHT.h"
#include <ESP32Servo.h>
#include <LittleFS.h>

// ── WiFi ────────────────────────────────────────────
const char *WIFI_SSID = "Virus";
const char *WIFI_PASSWORD = "3ng1n33r1ng";

// ── MQTT Broker (EMQX) ───────────────────────────────
const char *MQTT_HOST = "a14b5244.ala.eu-central-1.emqxsl.com";
const int MQTT_PORT = 8883;
const char *MQTT_USER = "musharukwa";
const char *MQTT_PASSWORD = "1@mt0077";
const char *MQTT_CLIENT = "esp32-brooder-1";

// ── MQTT Topics ──────────────────────────────────────
const char *TOPIC_COMMANDS = "brooder/1/commands";
const char *TOPIC_SENSORS = "brooder/1/sensors";
const char *TOPIC_STATUS = "brooder/1/status";

// ── Backend (REST for sensor history) ───────────────
const char *API_BASE = "https://flock-guardian-api.onrender.com/api/v1";
const char *PING_URL = "https://flock-guardian-api.onrender.com/health";
const int BROODER_ID = 1;

// ── Pin Definitions ─────────────────────────────────
#define TRIG_PIN_FEED 4
#define ECHO_PIN_FEED 2
#define TRIG_PIN_WATER 14
#define ECHO_PIN_WATER 12
#define DHTPIN 5
#define DHTTYPE DHT11
#define SERVO_PIN 18
#define FAN_PIN 27
#define PUMP_PIN 26
#define RELAY_ON LOW
#define RELAY_OFF HIGH

// ── Constants ───────────────────────────────────────
#define SERVO_CLOSED 0
#define SERVO_OPEN 90
#define CONTAINER_CM 30.0

// ── Automode Thresholds ─────────────────────────────
#define TEMP_HIGH 32.0      // °C — turn fan ON above this
#define TEMP_LOW 28.0       // °C — turn fan OFF below this
#define WATER_LOW_PCT 20.0  // % — turn pump ON below this
#define WATER_HIGH_PCT 80.0 // % — turn pump OFF above this
#define FEED_LOW_PCT 15.0   // % — open servo below this
#define FEED_HIGH_PCT 50.0  // % — close servo above this

// ── Offline / Automode ──────────────────────────────
#define OFFLINE_TIMEOUT_MS 300000UL // 5 minutes
#define PING_INTERVAL_MS 15000UL    // ping server every 15s
#define OFFLINE_LOG_FILE "/offline_log.json"

bool autoMode = false;
bool serverReachable = false;
unsigned long lastPingTime = 0;
unsigned long offlineSince = 0; // millis() when server first became unreachable
bool wasOffline = false;        // tracks transition from offline → online

//  Objects ─────────────────────────────────────────
DHT dht(DHTPIN, DHTTYPE);
Servo myServo;
WiFiClientSecure secureClient;
PubSubClient mqttClient(secureClient);

//  State ───────────────────────────────────────────
bool fanOn = false;
bool pumpOn = false;
bool servoOn = false;

//  Timers
unsigned long lastSensorPrint = 0;
unsigned long lastAPIPost = 0;
unsigned long lastMQTTPublish = 0;

const unsigned long SENSOR_INTERVAL = 2000;
const unsigned long API_INTERVAL = 10000;
const unsigned long MQTT_INTERVAL = 5000;
//  Sync state
unsigned long lastSyncAttempt = 0;
bool pendingSync = false;
const unsigned long SYNC_RETRY_INTERVAL = 60000UL;

//  Serial Buffer ───────────────────────────────────
String inputBuffer = "";

//  Helpers
//

long readUltrasonic(int trigPin, int echoPin)
{
    delay(30);
    digitalWrite(trigPin, LOW);
    delayMicroseconds(4);
    digitalWrite(trigPin, HIGH);
    delayMicroseconds(10);
    digitalWrite(trigPin, LOW);
    long duration = pulseIn(echoPin, HIGH, 30000);
    if (duration == 0)
        return CONTAINER_CM;
    return duration / 58.2;
}

float distanceToPercent(long cm)
{
    float pct = (1.0 - ((float)cm / CONTAINER_CM)) * 100.0;
    return constrain(pct, 0, 100);
}

// ────────────────────────────────────────────────────
//  LittleFS — Offline Log
// ────────────────────────────────────────────────────

void initFS()
{
    if (!LittleFS.begin(true))
    {
        Serial.println("[FS] LittleFS mount failed — formatting...");
        LittleFS.format();
        LittleFS.begin();
    }
    Serial.println("[FS] LittleFS ready");
}

// Append one sensor reading as a JSON line to the offline log file
void logOfflineReading(float temp, float hum, float feedPct, float waterPct)
{
    File f = LittleFS.open(OFFLINE_LOG_FILE, FILE_APPEND);
    if (!f)
    {
        Serial.println("[FS] Failed to open log for append");
        return;
    }

    StaticJsonDocument<200> doc;
    doc["temperature"] = temp;
    doc["humidity"] = hum;
    doc["feed_level"] = feedPct;
    doc["water_level"] = waterPct;
    doc["ts"] = millis(); // relative timestamp; replace with RTC if available

    String line;
    serializeJson(doc, line);
    f.println(line);
    f.close();

    Serial.println("[FS] Offline reading saved: " + line);
}

// Read offline log file and POST each line to the backend, then delete the file
void syncOfflineLog()
{
    if (!LittleFS.exists(OFFLINE_LOG_FILE))
    {
        Serial.println("[FS] No offline log to sync");
        pendingSync = false;
        return;
    }

    File f = LittleFS.open(OFFLINE_LOG_FILE, FILE_READ);
    if (!f)
    {
        Serial.println("[FS] Failed to open log — will retry");
        pendingSync = true;
        return;
    }

    DynamicJsonDocument doc(8192);
    JsonArray arr = doc.to<JsonArray>();

    while (f.available())
    {
        String line = f.readStringUntil('\n');
        line.trim();
        if (line.length() == 0)
            continue;
        StaticJsonDocument<200> entry;
        if (!deserializeJson(entry, line))
        {
            arr.add(entry.as<JsonObject>());
        }
    }
    f.close();

    if (arr.size() == 0)
    {
        LittleFS.remove(OFFLINE_LOG_FILE);
        pendingSync = false;
        return;
    }

    Serial.printf("[FS] Syncing %d readings...\n", arr.size());

    HTTPClient http;
    String url = String(API_BASE) + "/brooders/" + BROODER_ID + "/sensors/batch";
    http.begin(url);
    http.addHeader("Content-Type", "application/json");
    http.setTimeout(8000); // short timeout — don't hang long

    String body;
    serializeJson(doc, body);

    int code = http.POST(body);
    http.end();

    if (code > 0)
    {
        Serial.printf("[FS] Sync done — HTTP %d\n", code);
        LittleFS.remove(OFFLINE_LOG_FILE);
        Serial.println("[FS] Log deleted");
        pendingSync = false;
    }
    else
    {
        Serial.println("[FS] Sync failed — will retry in 60s");
        pendingSync = true;
        lastSyncAttempt = millis();
    }
}
    
void turnOffAll()
{
    if (fanOn)
    {
        fanOn = false;
        digitalWrite(FAN_PIN, RELAY_OFF);
    }
    if (pumpOn)
    {
        pumpOn = false;
        digitalWrite(PUMP_PIN, RELAY_OFF);
    }
    if (servoOn)
    {
        servoOn = false;
        myServo.write(SERVO_CLOSED);
    }
}
//
//  Connectivity Ping
//

bool pingServer()
{
    if (WiFi.status() != WL_CONNECTED)
        return false;

    HTTPClient http;
    http.begin(PING_URL);
    http.setTimeout(5000); // 5s timeout so it fails fast
    int code = http.GET();
    http.end();

    // Any HTTP response (even 4xx) means the server is reachable
    bool reachable = (code > 0);
    Serial.printf("[Ping] %s (HTTP %d)\n", reachable ? "Online" : "Offline", code);
    return reachable;
}

//
//  Automode Logic
//

void runAutoMode(float temp, float hum, float feedPct, float waterPct)
{
    // ── Temperature -- Fan
    if (!isnan(temp))
    {
        if (temp > TEMP_HIGH && !fanOn)
        {
            fanOn = true;
            digitalWrite(FAN_PIN, RELAY_ON);
            Serial.printf("[Auto] Temp %.1f°C > %.1f — Fan ON\n", temp, TEMP_HIGH);
        }
        else if (temp < TEMP_LOW && fanOn)
        {
            fanOn = false;
            digitalWrite(FAN_PIN, RELAY_OFF);
            Serial.printf("[Auto] Temp %.1f°C < %.1f — Fan OFF\n", temp, TEMP_LOW);
        }
    }

    // ── Water Level  Pump ────────────────────────
    if (waterPct < WATER_LOW_PCT && !pumpOn)
    {
        pumpOn = true;
        digitalWrite(PUMP_PIN, RELAY_ON);
        Serial.printf("[Auto] Water %.1f%% < %.1f%% — Pump ON\n", waterPct, WATER_LOW_PCT);
    }
    else if (waterPct > WATER_HIGH_PCT && pumpOn)
    {
        pumpOn = false;
        digitalWrite(PUMP_PIN, RELAY_OFF);
        Serial.printf("[Auto] Water %.1f%% > %.1f%% — Pump OFF\n", waterPct, WATER_HIGH_PCT);
    }

    // ── Feed Level → Servo ────────────────────────
    if (feedPct < FEED_LOW_PCT && !servoOn)
    {
        servoOn = true;
        myServo.write(SERVO_OPEN);
        Serial.printf("[Auto] Feed %.1f%% < %.1f%% — Servo OPEN\n", feedPct, FEED_LOW_PCT);
    }
    else if (feedPct > FEED_HIGH_PCT && servoOn)
    {
        servoOn = false;
        myServo.write(SERVO_CLOSED);
        Serial.printf("[Auto] Feed %.1f%% > %.1f%% — Servo CLOSED\n", feedPct, FEED_HIGH_PCT);
    }
}

//
//  Publish device status via MQTT
//

void publishStatus()
{
    StaticJsonDocument<192> doc;
    doc["fan"] = fanOn;
    doc["pump"] = pumpOn;
    doc["servo"] = servoOn;
    doc["autoMode"] = autoMode;
    String payload;
    serializeJson(doc, payload);
    mqttClient.publish(TOPIC_STATUS, payload.c_str(), true);
    Serial.println("[MQTT] Status published: " + payload);
}

//
//  Command Executor
//

void executeCommand(String cmd)
{
    cmd.trim();
    cmd.toLowerCase();

    if (cmd == "fan on")
    {
        fanOn = true;
        digitalWrite(FAN_PIN, RELAY_ON);
        Serial.println(">> Fan ON");
    }
    else if (cmd == "fan off")
    {
        fanOn = false;
        digitalWrite(FAN_PIN, RELAY_OFF);
        Serial.println(">> Fan OFF");
    }
    else if (cmd == "pump on")
    {
        pumpOn = true;
        digitalWrite(PUMP_PIN, RELAY_ON);
        Serial.println(">> Pump ON");
    }
    else if (cmd == "pump off")
    {
        pumpOn = false;
        digitalWrite(PUMP_PIN, RELAY_OFF);
        Serial.println(">> Pump OFF");
    }
    else if (cmd == "servo on")
    {
        servoOn = true;
        myServo.write(SERVO_OPEN);
        Serial.println(">> Servo OPEN");
    }
    else if (cmd == "servo off")
    {
        servoOn = false;
        myServo.write(SERVO_CLOSED);
        Serial.println(">> Servo CLOSED");
    }
    else if (cmd == "heater on")
    {
        Serial.println(">> Heater ON");
    }
    else if (cmd == "heater off")
    {
        Serial.println(">> Heater OFF");
    }
    else if (cmd == "auto on")
    {
        autoMode = true;
        Serial.println(">> AutoMode ENABLED (manual)");
    }
    else if (cmd == "auto off")
    {
        autoMode = false;
        Serial.println(">> AutoMode DISABLED (manual)");
    }
    else if (cmd == "status")
    {
        Serial.println("Fan: " + String(fanOn ? "ON" : "OFF"));
        Serial.println("Pump: " + String(pumpOn ? "ON" : "OFF"));
        Serial.println("Servo: " + String(servoOn ? "OPEN" : "CLOSED"));
        Serial.println("AutoMode: " + String(autoMode ? "ENABLED" : "DISABLED"));
        Serial.println("Server: " + String(serverReachable ? "Online" : "Offline"));
        Serial.print("IP: ");
        Serial.println(WiFi.localIP());
        Serial.println("MQTT: " + String(mqttClient.connected() ? "Connected" : "Disconnected"));
    }
    else
    {
        Serial.println("Unknown: " + cmd);
        return;
    }

    publishStatus();
}

//
//  MQTT Message Callback
//

void onMQTTMessage(char *topic, byte *payload, unsigned int length)
{
    String msg;
    for (unsigned int i = 0; i < length; i++)
        msg += (char)payload[i];
    Serial.println("[MQTT] Received on " + String(topic) + ": " + msg);

    StaticJsonDocument<128> doc;
    DeserializationError err = deserializeJson(doc, msg);
    if (!err && doc.containsKey("command"))
    {
        executeCommand(doc["command"].as<String>());
    }
    else
    {
        executeCommand(msg);
    }
}

//
//  WiFi
//

void connectWiFi()
{
    Serial.print("Connecting to WiFi");
    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 40)
    {
        delay(500);
        Serial.print(".");
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED)
    {
        Serial.println("\n[WiFi] Connected!");
        IPAddress ip = WiFi.localIP();
        IPAddress gateway = WiFi.gatewayIP();
        IPAddress subnet = WiFi.subnetMask();
        IPAddress dns1(8, 8, 8, 8);
        IPAddress dns2(1, 1, 1, 1);
        WiFi.config(ip, gateway, subnet, dns1, dns2);
        Serial.print("IP: ");
        Serial.println(ip);
    }
    else
    {
        Serial.println("\n[WiFi] Failed - offline mode");
    }
}

//
//  MQTT Connect
//

void connectMQTT()
{
    if (WiFi.status() != WL_CONNECTED)
    {
        Serial.println("[MQTT] No WiFi — skipping");
        return;
    }

    Serial.print("[MQTT] Connecting...");
    if (mqttClient.connect(MQTT_CLIENT, MQTT_USER, MQTT_PASSWORD))
    {
        Serial.println(" Connected! ✓");
        mqttClient.subscribe(TOPIC_COMMANDS);
        Serial.println("[MQTT] Subscribed to " + String(TOPIC_COMMANDS));
        publishStatus();
    }
    else
    {
        Serial.print(" Failed, rc=");
        Serial.println(mqttClient.state());
        Serial.println("[MQTT] Will retry in next loop");
    }
}

//
//  Post Sensor Data to Backend
//

void postSensorData(float temp, float hum, float feedPct, float waterPct)
{
    if (WiFi.status() != WL_CONNECTED)
        return;

    HTTPClient http;
    String url = String(API_BASE) + "/brooders/" + BROODER_ID + "/sensors";
    http.begin(url);
    http.addHeader("Content-Type", "application/json");

    StaticJsonDocument<200> doc;
    doc["temperature"] = temp;
    doc["humidity"] = hum;
    doc["feed_level"] = feedPct;
    doc["water_level"] = waterPct;

    String body;
    serializeJson(doc, body);
    int code = http.PATCH(body);
    if (code > 0)
        Serial.println("[API] Sent: " + String(code));
    else
        Serial.println("[API] Error: " + http.errorToString(code));
    http.end();
}

//
//  Setup
//

void setup()
{
    Serial.begin(115200);
    delay(1000);

    // Sensor pins
    pinMode(TRIG_PIN_FEED, OUTPUT);
    pinMode(ECHO_PIN_FEED, INPUT);
    pinMode(TRIG_PIN_WATER, OUTPUT);
    pinMode(ECHO_PIN_WATER, INPUT);

    // Relay pins
    pinMode(FAN_PIN, OUTPUT);
    pinMode(PUMP_PIN, OUTPUT);
    digitalWrite(FAN_PIN, RELAY_OFF);
    digitalWrite(PUMP_PIN, RELAY_OFF);

    // Peripherals
    dht.begin();
    myServo.attach(SERVO_PIN);
    myServo.write(SERVO_CLOSED);

    // Filesystem
    initFS();

    // Network
    connectWiFi();
    secureClient.setInsecure();
    mqttClient.setServer(MQTT_HOST, MQTT_PORT);
    mqttClient.setCallback(onMQTTMessage);
    mqttClient.setKeepAlive(60);
    mqttClient.setBufferSize(512);
    connectMQTT();

    // Initial ping to set baseline
    serverReachable = pingServer();
    if (serverReachable)
    {
        offlineSince = 0;
        wasOffline = false;
    }
    else
    {
        offlineSince = millis();
        wasOffline = true;
    }

    Serial.println("=== System Ready ===");
    Serial.println("Commands: fan on/off | pump on/off | servo on/off | auto on/off | status");
}

//
// Loop
//

void loop()
{
    
    if (WiFi.status() != WL_CONNECTED)
    {
        static unsigned long lastWifiRetry = 0;
        if (millis() - lastWifiRetry >= 30000)
        { // retry every 30s
            lastWifiRetry = millis();
            Serial.println("[WiFi] Disconnected — reconnecting...");
            WiFi.disconnect();
            WiFi.begin(WIFI_SSID, WIFI_PASSWORD);
        }
    }
    unsigned long now = millis();

    // ── Keep MQTT alive ──────────────────────────────
    if (WiFi.status() == WL_CONNECTED)
    {
        if (!mqttClient.connected())
            connectMQTT();
        mqttClient.loop();
    }

    // ── Serial Commands ──────────────────────────────
    while (Serial.available())
    {
        char c = Serial.read();
        if (c == '\n' || c == '\r')
        {
            if (inputBuffer.length())
            {
                executeCommand(inputBuffer);
                inputBuffer = "";
            }
        }
        else
        {
            inputBuffer += c;
        }
    }

    // ── Connectivity Ping ────────────────────────────
    if (now - lastPingTime >= PING_INTERVAL_MS)
    {
        lastPingTime = now;
        bool reachable = pingServer();

        if (reachable && !serverReachable)
        {
            turnOffAll();
            //Just came back online
            Serial.println("[Net] Server is back online!");
            serverReachable = true;

            if (wasOffline)
            {
                // Sync any data logged while offline
                syncOfflineLog();

                // Disable automode now that we're back
                if (autoMode)
                {
                    autoMode = false;
                    Serial.println("[Auto] AutoMode DISABLED — back online");
                    publishStatus();
                }
                wasOffline = false;
            }
            offlineSince = 0;
        }
        else if (!reachable && serverReachable)
        {
            //Just went offline
            Serial.println("[Net] Server unreachable — starting offline timer...");
            serverReachable = false;
            offlineSince = now;
        }
        else if (!reachable && !serverReachable)
        {
            // Still offline — check 5-min threshold
            if (!autoMode && (now - offlineSince >= OFFLINE_TIMEOUT_MS))
            {
                autoMode = true;
                wasOffline = true;
                Serial.println("[Auto] Offline for 5 min — AutoMode ENABLED");
                publishStatus(); // will likely fail but attempt anyway
            }
        }
    }
    // Retry pending sync 
    if (pendingSync && serverReachable &&
        millis() - lastSyncAttempt >= SYNC_RETRY_INTERVAL)
    {
        Serial.println("[FS] Retrying sync...");
        lastSyncAttempt = millis();
        syncOfflineLog();
    }

    // Sensor Reading every 2s 
    if (now - lastSensorPrint >= SENSOR_INTERVAL)
    {
        lastSensorPrint = now;

        long feedCm = readUltrasonic(TRIG_PIN_FEED, ECHO_PIN_FEED);
        long waterCm = readUltrasonic(TRIG_PIN_WATER, ECHO_PIN_WATER);
        float feedPct = distanceToPercent(feedCm);
        float waterPct = distanceToPercent(waterCm);
        float temp = dht.readTemperature();
        float hum = dht.readHumidity();

        // Print sensor readings
        Serial.print("[Sensor] ");
        if (isnan(temp) || isnan(hum))
        {
            Serial.print("DHT ERROR ");
        }
        else
        {
            Serial.print("T:");
            Serial.print(temp);
            Serial.print("C H:");
            Serial.print(hum);
            Serial.print("% ");
        }
        Serial.print("Feed:");
        Serial.print(feedPct);
        Serial.print("% Water:");
        Serial.print(waterPct);
        Serial.print("% | Auto:");
        Serial.println(autoMode ? "ON" : "OFF");

        // ── AutoMode: take over control 
        if (autoMode && !isnan(temp) && !isnan(hum))
        {
            runAutoMode(temp, hum, feedPct, waterPct);
        }

        //  Offline Logging 
        // Log every SENSOR_INTERVAL while in autoMode (offline)
        // Only log every 30s instead of every 2s while offline
        static unsigned long lastOfflineLog = 0;
        if (autoMode && !isnan(temp) && !isnan(hum))
        {
            if (millis() - lastOfflineLog >= 600000)
            {
                lastOfflineLog = millis();
                logOfflineReading(temp, hum, feedPct, waterPct);
            }
        }
        // ── Online: publish via MQTT every 5s ─────────
        if (!autoMode && (now - lastMQTTPublish >= MQTT_INTERVAL))
        {
            lastMQTTPublish = now;
            if (!isnan(temp) && !isnan(hum) && mqttClient.connected())
            {
                StaticJsonDocument<200> doc;
                doc["temperature"] = temp;
                doc["humidity"] = hum;
                doc["feed_level"] = feedPct;
                doc["water_level"] = waterPct;
                String payload;
                serializeJson(doc, payload);
                mqttClient.publish(TOPIC_SENSORS, payload.c_str());
                Serial.println("[MQTT] Sensors published");
            }
        }

        // ── Online: POST to REST API every 10s ─────────
        if (!autoMode && (now - lastAPIPost >= API_INTERVAL))
        {
            lastAPIPost = now;
            if (!isnan(temp) && !isnan(hum))
            {
                postSensorData(temp, hum, feedPct, waterPct);
            }
        }
    }
}
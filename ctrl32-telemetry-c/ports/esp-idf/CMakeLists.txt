idf_component_register(
    SRCS "ctrl32_telemetry_idf.c" "../../src/ctrl32_telemetry.c"
    INCLUDE_DIRS "." "../../src"
    REQUIRES mqtt esp_http_client app_update mbedtls nvs_flash esp_netif esp_timer esp-tls
)

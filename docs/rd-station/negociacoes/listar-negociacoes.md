Listar negociações

# Listar negociações

Listar negociações

# OpenAPI definition

```json
{
  "openapi": "3.1.1",
  "info": {
    "title": "RD Station CRM API v1",
    "version": "1.0",
    "description": "Documentação da API v1 do RD Station CRM.",
    "license": {
      "name": "Termos de Uso do Software RD Station CRM",
      "url": "https://legal.rdstation.com/pt/rdstation-crm-services-agreement/"
    },
    "termsOfService": "https://legal.rdstation.com/pt/rdstation-crm-services-agreement/",
    "contact": {
      "name": "Suporte RD Station",
      "url": "https://developers.rdstation.com/docs/suporte"
    }
  },
  "externalDocs": {
    "description": "Documentação completa",
    "url": "https://developers.rdstation.com/crm-v1"
  },
  "servers": [
    {
      "url": "https://crm.rdstation.com/api/v1",
      "description": "Production"
    }
  ],
  "security": [
    {
      "Token": []
    }
  ],
  "tags": [
    {
      "name": "crm-v1-deals",
      "description": "Negociações"
    }
  ],
  "paths": {
    "/deals": {
      "get": {
        "summary": "Listar negociações",
        "operationId": "crm-v1-list-deals",
        "tags": [
          "crm-v1-deals"
        ],
        "description": "Listar negociações",
        "parameters": [
          {
            "name": "page",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Número atual da página"
          },
          {
            "name": "limit",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Limite de negociações que serão listadas. Valor padrão é 20. Valor máximo é 200"
          },
          {
            "name": "order",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Ordenação. Valor padrão é \"created_at\""
          },
          {
            "name": "direction",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Ordenação da lista. \"asc\" ou \"desc\", padrão é \"desc\""
          },
          {
            "name": "name",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Nome da negociação. Para buscas com nome exato, usar o parâmetro exact_name=true"
          },
          {
            "name": "win",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Negociações ganhas. O valor true (retorna as negociações \"ganhas\"), false (retorna as negociações \"perdidas\") e null (retorna as negociações \"em aberto\")"
          },
          {
            "name": "user_id",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "ID do usuário relacionado à negociação"
          },
          {
            "name": "closed_at",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Ao informar true (retorna as negociações \"ganhas\" ou \"perdidas\"). Ao informar false (retorna as negociações \"em aberto\" OU \"pausadas\")"
          },
          {
            "name": "closed_at_period",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Data de fechamento da negociação: se true deve ser informado start_date e end_date"
          },
          {
            "name": "created_at_period",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Data de criação da negociação: se true, deve ser informado start_date e end_date"
          },
          {
            "name": "prediction_date_period",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Data de previsão de fechamento da negociação: se true, deve ser informado start_date e end_date"
          },
          {
            "name": "start_date",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Primeiro dia/hora em que deve ser aplicado o filtro para o parâmetro closed_at_period ou created_at_period. Ex.: \"start_date\": \"2020-12-14T15:00:00\""
          },
          {
            "name": "end_date",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Último dia/hora em que deve ser aplicado o filtro para o parâmetro closed_at_period ou created_at_period. Ex.: \"end_date\": \"2020-12-14T15:00:00\""
          },
          {
            "name": "campaign_id",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "ID da campanha"
          },
          {
            "name": "deal_stage_id",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "ID da etapa do funil de vendas"
          },
          {
            "name": "deal_lost_reason_id",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "ID do motivo de perda"
          },
          {
            "name": "deal_pipeline_id",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "ID do funil de vendas"
          },
          {
            "name": "organization",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "ID da empresa"
          },
          {
            "name": "hold",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Estado da negociação pausada. Se marcado como true (retorna todas negociações \"pausadas\"). Para outros casos, não deve-se utilizar esse parâmetro"
          },
          {
            "name": "product_presence",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "Negociações que contenham produtos/serviços relacionados. Se false (nenhum produto relacionado), true (um ou mais produtos relacionados) ou uma lista de IDs de produto. A lista de IDs deve ser informada com os valores separados por vírgula. Ex.: 5esdsds, d767dsdssd, c6e40fd2f000972a083"
          },
          {
            "name": "next_page",
            "in": "query",
            "schema": {
              "type": "string"
            },
            "description": "O parâmetro next_page serve para consultar a próxima página de resultados da busca corrente. Ele é obtido através da primeira consulta feita por esta API, porém todos os demais resultados apresentam este campo que se utilizado na requisição, navegam para sua próxima página."
          }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "examples": {
                "Listar negociações": {
                  "value": ""
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "deals": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "amount_montly": {
                            "type": "number",
                            "example": 0
                          },
                          "amount_total": {
                            "type": "number",
                            "example": 399.83
                          },
                          "amount_unique": {
                            "type": "number",
                            "example": 399.83
                          },
                          "closed_at": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
                          },
                          "contacts": {
                            "type": "array",
                            "items": {
                              "type": "object",
                              "properties": {
                                "birthday": {
                                  "type": "object",
                                  "properties": {
                                    "_id": {
                                      "type": "string",
                                      "example": "ID"
                                    },
                                    "created_at": {
                                      "type": [
                                        "string",
                                        "null"
                                      ],
                                      "format": "date-time"
                                    },
                                    "day": {
                                      "type": "number",
                                      "example": 11
                                    },
                                    "month": {
                                      "type": "number",
                                      "example": 9
                                    },
                                    "updated_at": {
                                      "type": [
                                        "string",
                                        "null"
                                      ],
                                      "format": "date-time"
                                    },
                                    "year": {
                                      "type": "number",
                                      "example": 1979
                                    }
                                  }
                                },
                                "emails": {
                                  "type": "array",
                                  "items": {
                                    "type": "object",
                                    "properties": {
                                      "email": {
                                        "type": "string",
                                        "example": "email1@empresa.com"
                                      }
                                    }
                                  }
                                },
                                "facebook": {
                                  "type": [
                                    "string",
                                    "null"
                                  ]
                                },
                                "linkedin": {
                                  "type": [
                                    "string",
                                    "null"
                                  ]
                                },
                                "name": {
                                  "type": "string",
                                  "example": "NAME"
                                },
                                "notes": {
                                  "type": [
                                    "string",
                                    "null"
                                  ]
                                },
                                "phones": {
                                  "type": "array",
                                  "items": {
                                    "type": "object",
                                    "properties": {
                                      "phone": {
                                        "type": "string",
                                        "example": "3165585457"
                                      },
                                      "type": {
                                        "type": "string",
                                        "example": "cellphone"
                                      }
                                    }
                                  }
                                },
                                "skype": {
                                  "type": "string",
                                  "example": "EMAIL"
                                },
                                "title": {
                                  "type": "string",
                                  "example": "Gerente"
                                }
                              }
                            }
                          },
                          "created_at": {
                            "type": "string",
                            "example": "2023-02-16T10:29:36.600-03:00"
                          },
                          "deal_custom_fields": {
                            "type": "array",
                            "items": {}
                          },
                          "deal_products": {
                            "type": "array",
                            "items": {
                              "type": "object",
                              "properties": {
                                "_id": {
                                  "type": "string",
                                  "example": "ID"
                                },
                                "amount": {
                                  "type": "number",
                                  "example": 1
                                },
                                "amount_decimal": {
                                  "type": "number",
                                  "example": 1.1
                                },
                                "base_price": {
                                  "type": "number",
                                  "example": 199.91
                                },
                                "created_at": {
                                  "type": "string",
                                  "example": "2023-02-16T10:29:36.778-03:00"
                                },
                                "description": {
                                  "type": "string",
                                  "example": "Description Product 1"
                                },
                                "discount": {
                                  "type": "number",
                                  "example": 0
                                },
                                "discount_type": {
                                  "type": "string",
                                  "example": "value"
                                },
                                "id": {
                                  "type": "string",
                                  "example": "ID"
                                },
                                "name": {
                                  "type": "string",
                                  "example": "Product 01"
                                },
                                "price": {
                                  "type": "number",
                                  "example": 199.91
                                },
                                "product_id": {
                                  "type": "string",
                                  "example": "ID"
                                },
                                "recurrence": {
                                  "type": "string",
                                  "example": "spare"
                                },
                                "total": {
                                  "type": "number",
                                  "example": 199.91
                                },
                                "updated_at": {
                                  "type": "string",
                                  "example": "2023-02-16T10:29:36.778-03:00"
                                }
                              }
                            }
                          },
                          "deal_stage": {
                            "type": "object",
                            "properties": {
                              "_id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "created_at": {
                                "type": "string",
                                "example": "2022-04-26T16:17:58.214-03:00"
                              },
                              "id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "name": {
                                "type": "string",
                                "example": "Sem contato"
                              },
                              "nickname": {
                                "type": "string",
                                "example": "SC"
                              },
                              "updated_at": {
                                "type": "string",
                                "example": "2022-04-26T16:17:58.214-03:00"
                              }
                            }
                          },
                          "hold": {
                            "type": [
                              "boolean",
                              "null"
                            ]
                          },
                          "id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "interactions": {
                            "type": "number",
                            "example": 0
                          },
                          "last_activity_at": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
                          },
                          "last_activity_content": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "markup": {
                            "type": "string",
                            "example": "future"
                          },
                          "markup_created": {
                            "type": "string",
                            "example": "8 days"
                          },
                          "markup_last_activities": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "name": {
                            "type": "string",
                            "example": "nome da oportunidade 1"
                          },
                          "prediction_date": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date"
                          },
                          "rating": {
                            "type": "number",
                            "example": 1
                          },
                          "stop_time_limit": {
                            "type": "object",
                            "properties": {}
                          },
                          "updated_at": {
                            "type": "string",
                            "example": "2023-02-16T10:30:05.171-03:00"
                          },
                          "user": {
                            "type": "object",
                            "properties": {
                              "_id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "email": {
                                "type": "string",
                                "example": "EMAIL"
                              },
                              "id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "name": {
                                "type": "string",
                                "example": "NAME"
                              },
                              "nickname": {
                                "type": "string",
                                "example": "RP"
                              }
                            }
                          },
                          "user_changed": {
                            "type": "boolean",
                            "example": true
                          },
                          "win": {
                            "type": [
                              "boolean",
                              "null"
                            ]
                          }
                        }
                      }
                    },
                    "has_more": {
                      "type": "boolean",
                      "example": false
                    },
                    "next_page": {
                      "type": "string",
                      "example": "i-1726778672521,s-66ec8d30f2f11000139ab5ff"
                    },
                    "total": {
                      "type": "number",
                      "example": 1
                    }
                  }
                },
                "examples": {
                  "Success": {
                    "value": {
                      "deals": [
                        {
                          "_id": "ID",
                          "amount_montly": 0,
                          "amount_total": 399.83,
                          "amount_unique": 399.83,
                          "closed_at": null,
                          "contacts": [
                            {
                              "birthday": {
                                "_id": "ID",
                                "created_at": null,
                                "day": 11,
                                "month": 9,
                                "updated_at": null,
                                "year": 1979
                              },
                              "emails": [
                                {
                                  "email": "email1@empresa.com"
                                },
                                {
                                  "email": "email2@empresa.com"
                                }
                              ],
                              "facebook": null,
                              "linkedin": null,
                              "name": "NAME",
                              "notes": null,
                              "phones": [
                                {
                                  "phone": "3165585457",
                                  "type": "cellphone"
                                }
                              ],
                              "skype": "EMAIL",
                              "title": "Gerente"
                            }
                          ],
                          "created_at": "2023-02-16T10:29:36.600-03:00",
                          "deal_custom_fields": [],
                          "deal_products": [
                            {
                              "_id": "ID",
                              "amount": 1,
                              "amount_decimal": 1.1,
                              "base_price": 199.91,
                              "created_at": "2023-02-16T10:29:36.778-03:00",
                              "description": "Description Product 1",
                              "discount": 0,
                              "discount_type": "value",
                              "id": "ID",
                              "name": "Product 01",
                              "price": 199.91,
                              "product_id": "ID",
                              "recurrence": "spare",
                              "total": 199.91,
                              "updated_at": "2023-02-16T10:29:36.778-03:00"
                            },
                            {
                              "_id": "ID",
                              "amount": 1,
                              "amount_decimal": 1.1,
                              "base_price": 199.92,
                              "created_at": "2023-02-16T10:29:36.816-03:00",
                              "description": "Description Product 2",
                              "discount": 0,
                              "discount_type": "value",
                              "id": "ID",
                              "name": "Product 02",
                              "price": 199.92,
                              "product_id": "ID",
                              "recurrence": "spare",
                              "total": 199.92,
                              "updated_at": "2023-02-16T10:29:36.816-03:00"
                            }
                          ],
                          "deal_stage": {
                            "_id": "ID",
                            "created_at": "2022-04-26T16:17:58.214-03:00",
                            "id": "ID",
                            "name": "Sem contato",
                            "nickname": "SC",
                            "updated_at": "2022-04-26T16:17:58.214-03:00"
                          },
                          "hold": null,
                          "id": "ID",
                          "interactions": 0,
                          "last_activity_at": null,
                          "last_activity_content": null,
                          "markup": "future",
                          "markup_created": "8 days",
                          "markup_last_activities": null,
                          "name": "nome da oportunidade 1",
                          "prediction_date": null,
                          "rating": 1,
                          "stop_time_limit": {},
                          "updated_at": "2023-02-16T10:30:05.171-03:00",
                          "user": {
                            "_id": "ID",
                            "email": "EMAIL",
                            "id": "ID",
                            "name": "NAME",
                            "nickname": "RP"
                          },
                          "user_changed": true,
                          "win": null
                        }
                      ],
                      "has_more": false,
                      "next_page": "i-1726778672521,s-66ec8d30f2f11000139ab5ff",
                      "total": 1
                    }
                  }
                }
              }
            }
          },
          "400": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "error": {
                      "type": "string",
                      "example": "Invalid next page param format"
                    }
                  }
                },
                "examples": {
                  "Error": {
                    "value": {
                      "error": "Invalid next page param format"
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "Token": {
        "type": "apiKey",
        "name": "token",
        "in": "query",
        "description": "Token do usuário"
      }
    }
  }
}
```
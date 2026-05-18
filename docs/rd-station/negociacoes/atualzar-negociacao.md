Atualizar negociação

# Atualizar negociação

Atualizar negociação

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
    "/deals/{deal_id}": {
      "put": {
        "summary": "Atualizar negociação",
        "operationId": "crm-v1-update-deal",
        "tags": [
          "crm-v1-deals"
        ],
        "description": "Atualizar negociação",
        "parameters": [
          {
            "name": "deal_id",
            "in": "path",
            "required": true,
            "description": "ID da negociação.",
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "campaign": {
                    "type": "object",
                    "properties": {
                      "_id": {
                        "type": "string",
                        "example": "63d83e4f8cc2e6000f8e91c8"
                      }
                    }
                  },
                  "deal": {
                    "type": "object",
                    "properties": {
                      "deal_custom_fields": {
                        "type": "array",
                        "items": {
                          "type": "object",
                          "properties": {
                            "custom_field_id": {
                              "type": "string",
                              "example": "63c188a315b034000fbf62d1"
                            },
                            "value": {
                              "example": "Opção A",
                              "anyOf": [
                                {
                                  "type": [
                                    "string",
                                    "null"
                                  ],
                                  "example": "Opção A"
                                },
                                {
                                  "type": "array",
                                  "items": {
                                    "type": "string",
                                    "example": "Opção 1"
                                  }
                                }
                              ]
                            }
                          }
                        }
                      },
                      "deal_lost_note": {
                        "type": [
                          "string",
                          "null"
                        ]
                      },
                      "deal_lost_reason_id": {
                        "type": [
                          "string",
                          "null"
                        ]
                      },
                      "hold": {
                        "type": [
                          "boolean",
                          "null"
                        ]
                      },
                      "name": {
                        "type": "string",
                        "example": "Negociação Atualizada"
                      },
                      "organization_id": {
                        "type": "string",
                        "example": "64e002800e06f5000dad7673"
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
                      "user_id": {
                        "type": "string",
                        "example": "5d1e122a572a6c0034f2e595"
                      },
                      "win": {
                        "type": [
                          "boolean",
                          "null"
                        ]
                      }
                    }
                  },
                  "deal_source": {
                    "type": "object",
                    "properties": {
                      "_id": {
                        "type": "string",
                        "example": "61140ea5c794c70001b71985"
                      }
                    }
                  },
                  "deal_stage_id": {
                    "type": "string",
                    "example": "633de55c96c4ac0019c8a638"
                  }
                }
              },
              "examples": {
                "Atualizar negociação": {
                  "value": {
                    "campaign": {
                      "_id": "63d83e4f8cc2e6000f8e91c8"
                    },
                    "deal": {
                      "deal_custom_fields": [
                        {
                          "custom_field_id": "63c188a315b034000fbf62d1",
                          "value": "Opção A"
                        },
                        {
                          "custom_field_id": "63e55e890ef6e0001819ddee",
                          "value": [
                            "Opção 1",
                            "Opção 2"
                          ]
                        }
                      ],
                      "deal_lost_note": null,
                      "deal_lost_reason_id": null,
                      "hold": null,
                      "name": "Negociação Atualizada",
                      "organization_id": "64e002800e06f5000dad7673",
                      "prediction_date": null,
                      "rating": 1,
                      "user_id": "5d1e122a572a6c0034f2e595",
                      "win": null
                    },
                    "deal_source": {
                      "_id": "61140ea5c794c70001b71985"
                    },
                    "deal_stage_id": "633de55c96c4ac0019c8a638"
                  }
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
                    "best_moment_to_touch": {
                      "type": "boolean",
                      "example": false
                    },
                    "c_cf_errors": {
                      "type": "object",
                      "properties": {}
                    },
                    "campaign": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Campanha A"
                        }
                      }
                    },
                    "campaign_id": {
                      "type": "string",
                      "example": "ID"
                    },
                    "closed_at": {
                      "type": [
                        "string",
                        "null"
                      ],
                      "format": "date-time"
                    },
                    "contact_errors": {
                      "type": "object",
                      "properties": {}
                    },
                    "created_at": {
                      "type": "string",
                      "example": "2023-08-30T21:55:50.376-03:00"
                    },
                    "deal_custom_fields": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "created_at": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
                          },
                          "custom_field": {
                            "type": "object",
                            "properties": {
                              "_id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "allow_new": {
                                "type": "boolean",
                                "example": true
                              },
                              "created_at": {
                                "type": "string",
                                "example": "2023-01-13T13:36:51.156-03:00"
                              },
                              "for": {
                                "type": "string",
                                "example": "deal"
                              },
                              "instance_id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "label": {
                                "type": "string",
                                "example": "Assuntos relacionados"
                              },
                              "opts": {
                                "type": "array",
                                "items": {
                                  "type": "string",
                                  "example": "Assunto 2"
                                }
                              },
                              "order": {
                                "type": "number",
                                "example": 1
                              },
                              "required": {
                                "type": "boolean",
                                "example": false
                              },
                              "required_rules": {
                                "type": "array",
                                "items": {
                                  "type": "object",
                                  "properties": {
                                    "_id": {
                                      "type": "string",
                                      "example": "64f09657eb9e69000dcfd3ca"
                                    },
                                    "always": {
                                      "type": [
                                        "boolean",
                                        "null"
                                      ]
                                    },
                                    "created_at": {
                                      "type": "string",
                                      "example": "2023-08-31T10:32:07.309-03:00"
                                    },
                                    "property": {
                                      "type": "string",
                                      "example": "deal_stage_id"
                                    },
                                    "updated_at": {
                                      "type": "string",
                                      "example": "2023-08-31T10:32:07.309-03:00"
                                    },
                                    "value": {
                                      "type": "string",
                                      "example": "5c911f5221fdf40026e2ae9b"
                                    }
                                  }
                                }
                              },
                              "type": {
                                "type": "string",
                                "example": "option"
                              },
                              "unique": {
                                "type": "boolean",
                                "example": false
                              },
                              "updated_at": {
                                "type": "string",
                                "example": "2023-08-30T21:51:06.034-03:00"
                              },
                              "visible": {
                                "type": "boolean",
                                "example": false
                              }
                            }
                          },
                          "custom_field_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "updated_at": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
                          },
                          "value": {
                            "example": "Opção A",
                            "anyOf": [
                              {
                                "type": "string",
                                "example": "Opção A"
                              },
                              {
                                "type": "array",
                                "items": {
                                  "type": "string",
                                  "example": "Opção 1"
                                }
                              },
                              {
                                "type": [
                                  "string",
                                  "null"
                                ]
                              }
                            ]
                          }
                        }
                      }
                    },
                    "deal_lost_note": {
                      "type": [
                        "string",
                        "null"
                      ]
                    },
                    "deal_lost_reason_id": {
                      "type": [
                        "string",
                        "null"
                      ]
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
                          "base_price": {
                            "type": "number",
                            "example": 199.91
                          },
                          "created_at": {
                            "type": "string",
                            "example": "2023-08-30T21:55:50.570-03:00"
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
                            "example": "2023-08-30T21:55:50.570-03:00"
                          }
                        }
                      }
                    },
                    "deal_source": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "3.0"
                        }
                      }
                    },
                    "deal_source_id": {
                      "type": "string",
                      "example": "ID"
                    },
                    "deal_stage": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "deal_pipeline_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Converteu"
                        },
                        "nickname": {
                          "type": "string",
                          "example": "C"
                        }
                      }
                    },
                    "deal_stage_histories": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "deal_stage_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "end_date": {
                            "type": [
                              "string",
                              "null"
                            ]
                          },
                          "id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "start_date": {
                            "type": "string",
                            "example": "2023-09-05T18:25:40.421-03:00"
                          }
                        }
                      }
                    },
                    "errors": {
                      "type": "object",
                      "properties": {}
                    },
                    "from_rdsm_integration": {
                      "type": "boolean",
                      "example": false
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
                      "example": 11
                    },
                    "last_note_content": {
                      "type": [
                        "string",
                        "null"
                      ]
                    },
                    "name": {
                      "type": "string",
                      "example": "Negociação Atualizada"
                    },
                    "organization": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "address": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "address_latitude": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "address_longitude": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Empresa X"
                        },
                        "organization_custom_fields": {
                          "type": "array",
                          "items": {
                            "type": "object",
                            "properties": {
                              "_id": {
                                "type": "string",
                                "example": "64e002800e06f5000dad7665"
                              },
                              "created_at": {
                                "type": [
                                  "string",
                                  "null"
                                ],
                                "format": "date-time"
                              },
                              "custom_field_id": {
                                "type": "string",
                                "example": "646e73a3651e44000f352486"
                              },
                              "updated_at": {
                                "type": [
                                  "string",
                                  "null"
                                ],
                                "format": "date-time"
                              },
                              "value": {
                                "anyOf": [
                                  {
                                    "type": [
                                      "string",
                                      "null"
                                    ]
                                  },
                                  {
                                    "type": "array",
                                    "items": {}
                                  }
                                ]
                              }
                            }
                          }
                        },
                        "organization_segments": {
                          "type": "array",
                          "items": {}
                        },
                        "resume": {
                          "type": "string"
                        },
                        "url": {
                          "type": "string"
                        },
                        "visible": {
                          "type": "boolean",
                          "example": true
                        }
                      }
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
                      "properties": {
                        "expiration_date_time": {
                          "type": "string",
                          "example": "2026-05-31T18:25:40.533-03:00"
                        },
                        "expired": {
                          "type": "boolean",
                          "example": false
                        },
                        "expired_days": {
                          "type": "number",
                          "example": 0
                        }
                      }
                    },
                    "updated_at": {
                      "type": "string",
                      "example": "2023-09-05T18:25:40.534-03:00"
                    },
                    "user": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Tays Teixeira"
                        }
                      }
                    },
                    "win": {
                      "type": [
                        "boolean",
                        "null"
                      ]
                    }
                  }
                },
                "examples": {
                  "Success": {
                    "value": {
                      "_id": "ID",
                      "amount_montly": 0,
                      "amount_total": 399.83,
                      "amount_unique": 399.83,
                      "best_moment_to_touch": false,
                      "c_cf_errors": {},
                      "campaign": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "Campanha A"
                      },
                      "campaign_id": "ID",
                      "closed_at": null,
                      "contact_errors": {},
                      "created_at": "2023-08-30T21:55:50.376-03:00",
                      "deal_custom_fields": [
                        {
                          "created_at": null,
                          "custom_field": {
                            "_id": "ID",
                            "allow_new": true,
                            "created_at": "2023-01-13T13:36:51.156-03:00",
                            "for": "deal",
                            "instance_id": "ID",
                            "label": "Assuntos relacionados",
                            "opts": [
                              "Assunto 2",
                              "Assunto 3",
                              "Opção A"
                            ],
                            "order": 1,
                            "required": false,
                            "type": "option",
                            "unique": false,
                            "updated_at": "2023-08-30T21:51:06.034-03:00",
                            "visible": false
                          },
                          "custom_field_id": "ID",
                          "updated_at": null,
                          "value": "Opção A"
                        },
                        {
                          "created_at": null,
                          "custom_field": {
                            "_id": "ID",
                            "allow_new": true,
                            "created_at": "2023-02-09T17:58:49.592-03:00",
                            "for": "deal",
                            "instance_id": "ID",
                            "label": "Tags",
                            "opts": [
                              "tag 1",
                              "tag 2",
                              "nome da tag",
                              "leads",
                              "[\"tag 1\"]",
                              "Opção 6 & 6",
                              "caique 2",
                              "caique 1",
                              "ana banana da silva",
                              "Opção 1",
                              "Opção 2"
                            ],
                            "order": 3,
                            "required": false,
                            "type": "multiple_choice",
                            "unique": false,
                            "updated_at": "2023-08-30T21:50:44.612-03:00",
                            "visible": false
                          },
                          "custom_field_id": "ID",
                          "updated_at": null,
                          "value": [
                            "Opção 1",
                            "Opção 2"
                          ]
                        },
                        {
                          "created_at": null,
                          "custom_field": {
                            "_id": "ID",
                            "allow_new": true,
                            "created_at": "2023-08-31T10:32:07.309-03:00",
                            "for": "deal",
                            "instance_id": "ID",
                            "label": "Teste obrigatoriedade",
                            "opts": [
                              "Opção 1",
                              "Opção 2",
                              "Opção 3"
                            ],
                            "order": 31,
                            "required": true,
                            "required_rules": [
                              {
                                "_id": "64f09657eb9e69000dcfd3ca",
                                "always": null,
                                "created_at": "2023-08-31T10:32:07.309-03:00",
                                "property": "deal_stage_id",
                                "updated_at": "2023-08-31T10:32:07.309-03:00",
                                "value": "5c911f5221fdf40026e2ae9b"
                              },
                              {
                                "_id": "64f09657eb9e69000dcfd3cb",
                                "always": null,
                                "created_at": "2023-08-31T10:32:07.309-03:00",
                                "property": "deal_stage_id",
                                "updated_at": "2023-08-31T10:32:07.309-03:00",
                                "value": "633dcb7b2d6d4000115010b9"
                              },
                              {
                                "_id": "64f09657eb9e69000dcfd3cc",
                                "always": null,
                                "created_at": "2023-08-31T10:32:07.309-03:00",
                                "property": "deal_stage_id",
                                "updated_at": "2023-08-31T10:32:07.309-03:00",
                                "value": "64ed37bda20856000d7a3b9a"
                              },
                              {
                                "_id": "64f09657eb9e69000dcfd3cd",
                                "always": null,
                                "created_at": "2023-08-31T10:32:07.309-03:00",
                                "property": "deal_stage_id",
                                "updated_at": "2023-08-31T10:32:07.309-03:00",
                                "value": "64d13fe07de18f002b92c341"
                              },
                              {
                                "_id": "64f09657eb9e69000dcfd3ce",
                                "always": null,
                                "created_at": "2023-08-31T10:32:07.309-03:00",
                                "property": "deal_stage_id",
                                "updated_at": "2023-08-31T10:32:07.309-03:00",
                                "value": "64d1461a20357e0017719ca7"
                              }
                            ],
                            "type": "multiple_choice",
                            "unique": false,
                            "updated_at": "2023-08-31T10:32:07.309-03:00",
                            "visible": true
                          },
                          "custom_field_id": "ID",
                          "updated_at": null,
                          "value": null
                        }
                      ],
                      "deal_lost_note": null,
                      "deal_lost_reason_id": null,
                      "deal_products": [
                        {
                          "_id": "ID",
                          "amount": 1,
                          "base_price": 199.91,
                          "created_at": "2023-08-30T21:55:50.570-03:00",
                          "description": "Description Product 1",
                          "discount": 0,
                          "discount_type": "value",
                          "id": "ID",
                          "name": "Product 01",
                          "price": 199.91,
                          "product_id": "ID",
                          "recurrence": "spare",
                          "total": 199.91,
                          "updated_at": "2023-08-30T21:55:50.570-03:00"
                        },
                        {
                          "_id": "ID",
                          "amount": 1,
                          "base_price": 199.92,
                          "created_at": "2023-08-30T21:55:50.602-03:00",
                          "description": "Description Product 2",
                          "discount": 0,
                          "discount_type": "value",
                          "id": "ID",
                          "name": "Product 02",
                          "price": 199.92,
                          "product_id": "ID",
                          "recurrence": "spare",
                          "total": 199.92,
                          "updated_at": "2023-08-30T21:55:50.602-03:00"
                        }
                      ],
                      "deal_source": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "3.0"
                      },
                      "deal_source_id": "ID",
                      "deal_stage": {
                        "_id": "ID",
                        "deal_pipeline_id": "ID",
                        "id": "ID",
                        "name": "Converteu",
                        "nickname": "C"
                      },
                      "deal_stage_histories": [
                        {
                          "_id": "ID",
                          "deal_stage_id": "ID",
                          "end_date": null,
                          "id": "ID",
                          "start_date": "2023-09-05T18:25:40.421-03:00"
                        },
                        {
                          "_id": "ID",
                          "deal_stage_id": "ID",
                          "end_date": null,
                          "id": "ID",
                          "start_date": "2023-09-05T18:25:40.421-03:00"
                        },
                        {
                          "_id": "ID",
                          "deal_stage_id": "ID",
                          "end_date": "2023-09-05T18:25:40.421-03:00",
                          "id": "ID",
                          "start_date": "2023-09-05T18:25:18.793-03:00"
                        }
                      ],
                      "errors": {},
                      "from_rdsm_integration": false,
                      "hold": null,
                      "id": "ID",
                      "interactions": 11,
                      "last_note_content": null,
                      "name": "Negociação Atualizada",
                      "organization": {
                        "_id": "ID",
                        "address": null,
                        "address_latitude": null,
                        "address_longitude": null,
                        "id": "ID",
                        "name": "Empresa X",
                        "organization_custom_fields": [
                          {
                            "_id": "64e002800e06f5000dad7665",
                            "created_at": null,
                            "custom_field_id": "646e73a3651e44000f352486",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7666",
                            "created_at": null,
                            "custom_field_id": "646e7ca7a204a9001025fba3",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7667",
                            "created_at": null,
                            "custom_field_id": "645e82e6a7622b001266d7d4",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7668",
                            "created_at": null,
                            "custom_field_id": "645e8301edc7f700122d9fde",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7674",
                            "created_at": null,
                            "custom_field_id": "645e838876cda400186c82c2",
                            "updated_at": null,
                            "value": []
                          },
                          {
                            "_id": "64e002800e06f5000dad7669",
                            "created_at": null,
                            "custom_field_id": "63dc2e47e94f2500155126d8",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad766e",
                            "created_at": null,
                            "custom_field_id": "6467bcba808e0f0012a3b2d5",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad766a",
                            "created_at": null,
                            "custom_field_id": "63e694e5949776002887d6e2",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad766b",
                            "created_at": null,
                            "custom_field_id": "646e67355e31ea00136a3638",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad766c",
                            "created_at": null,
                            "custom_field_id": "647f95e72414260017ca301c",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad766d",
                            "created_at": null,
                            "custom_field_id": "648092a87c18a1001896d3ab",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7675",
                            "created_at": null,
                            "custom_field_id": "6499f74d1e8a78000f3770de",
                            "updated_at": null,
                            "value": null
                          },
                          {
                            "_id": "64e002800e06f5000dad766f",
                            "created_at": null,
                            "custom_field_id": "64bfc8320f419c0019d9910d",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7670",
                            "created_at": null,
                            "custom_field_id": "64c0154a8fd1af000f9c20d0",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7671",
                            "created_at": null,
                            "custom_field_id": "64c3f397a657df002cbef8f5",
                            "updated_at": null,
                            "value": ""
                          },
                          {
                            "_id": "64e002800e06f5000dad7672",
                            "created_at": null,
                            "custom_field_id": "64cda7bc7e0513000e42b3f8",
                            "updated_at": null,
                            "value": ""
                          }
                        ],
                        "organization_segments": [],
                        "resume": "",
                        "url": "",
                        "visible": true
                      },
                      "prediction_date": null,
                      "rating": 1,
                      "stop_time_limit": {
                        "expiration_date_time": "2026-05-31T18:25:40.533-03:00",
                        "expired": false,
                        "expired_days": 0
                      },
                      "updated_at": "2023-09-05T18:25:40.534-03:00",
                      "user": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "Tays Teixeira"
                      },
                      "win": null
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